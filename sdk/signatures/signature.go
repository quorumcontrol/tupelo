package signatures

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/messages/v2/build/go/signatures"

	logging "github.com/ipfs/go-log"
	"github.com/quorumcontrol/tupelo/sdk/bls"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/xerrors"

	"github.com/spy16/parens"
	"github.com/spy16/parens/stdlib"
)

var logger = logging.Logger("signatures")

var defaultScope parens.Scope

type entry struct {
	key string
	val interface{}
}

func bindMany(scope parens.Scope, entries []entry) error {
	var err error
	for _, entry := range entries {
		err = scope.Bind(entry.key, entry.val)
		if err != nil {
			return xerrors.Errorf("error binding: %w", err)
		}
	}
	return nil
}

func init() {
	scope := parens.NewScope(nil)
	err := bindMany(scope, []entry{
		{"cond", parens.MacroFunc(stdlib.Conditional)},
		{"true", true},
		{"false", false},
		{"nil", false},
		{"println", func(str string) {
			fmt.Println(str)
		}},
		{"now", func() int64 {
			return time.Now().UTC().Unix()
		}},
	})
	if err != nil {
		panic(err)
	}
	err = stdlib.RegisterMath(scope)
	if err != nil {
		panic(err)
	}
	defaultScope = scope
}

var nullAddr = common.BytesToAddress([]byte{})

func Address(o *signatures.Ownership) (common.Address, error) {
	// in the case of conditions, all signatures are treated similarly to produce an address
	// and we just take the hash of the public key and the conditions and produce an address
	if o.Conditions != "" {
		pubKeyWithConditions := append(o.PublicKey.PublicKey, []byte(o.Conditions)...)
		return bytesToAddress(pubKeyWithConditions), nil
	}

	switch o.PublicKey.Type {
	case signatures.PublicKey_KeyTypeSecp256k1:
		key, err := crypto.UnmarshalPubkey(o.PublicKey.PublicKey)
		if err != nil {
			return nullAddr, xerrors.Errorf("error unmarshaling public key: %w", err)
		}
		return crypto.PubkeyToAddress(*key), nil
	case signatures.PublicKey_KeyTypeBLSGroupSig:
		return bytesToAddress(o.PublicKey.PublicKey), nil
	default:
		return nullAddr, xerrors.Errorf("unknown keytype: %s", o.PublicKey.Type)
	}
}

func bytesToAddress(bits []byte) common.Address {
	return common.BytesToAddress(crypto.Keccak256(bits))
}

func RestoreEcdsaPublicKey(ctx context.Context, s *signatures.Signature, hsh []byte) error {
	sp, _ := opentracing.StartSpanFromContext(ctx, "signatures.RestoreEcdsaPublicKey")
	defer sp.Finish()

	if s.Ownership.PublicKey.Type != signatures.PublicKey_KeyTypeSecp256k1 {
		return xerrors.Errorf("error only KeyTypeSecp256k1 supports key recovery")
	}
	recoveredPub, err := crypto.SigToPub(hsh, s.Signature)
	if err != nil {
		return xerrors.Errorf("error recovering signature: %w", err)
	}
	s.Ownership.PublicKey.PublicKey = crypto.FromECDSAPub(recoveredPub)
	return nil
}

func RestoreBLSPublicKey(ctx context.Context, s *signatures.Signature, knownVerKeys []*bls.VerKey) error {
	sp, _ := opentracing.StartSpanFromContext(ctx, "signatures.RestoreBLSPublicKey")
	defer sp.Finish()

	if len(knownVerKeys) != len(s.Signers) {
		return xerrors.Errorf("error known verkeys length did not match signers length: %d != %d", len(knownVerKeys), len(s.Signers))
	}
	var verKeys []*bls.VerKey

	var signerCount uint64
	for i, cnt := range s.Signers {
		if cnt > 0 {
			signerCount++
			verKey := knownVerKeys[i]
			newKeys := make([]*bls.VerKey, cnt)
			for j := uint32(0); j < cnt; j++ {
				newKeys[j] = verKey
			}
			verKeys = append(verKeys, newKeys...)
		}
	}
	key, err := bls.SumVerKeys(verKeys)
	if err != nil {
		return xerrors.Errorf("error summing keys: %w", err)
	}
	s.Ownership.PublicKey = &signatures.PublicKey{
		Type:      signatures.PublicKey_KeyTypeBLSGroupSig,
		PublicKey: key.Bytes(),
	}
	return nil
}

func Valid(ctx context.Context, s *signatures.Signature, hsh []byte, scope parens.Scope) (bool, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "signatures.Valid")
	defer sp.Finish()

	if len(s.Ownership.PublicKey.PublicKey) == 0 {
		return false, xerrors.Errorf("public key was 0, perhaps you forgot to restore it from sig?")
	}

	conditionsValid, err := validConditions(ctx, s, scope)
	if err != nil {
		return false, xerrors.Errorf("error validating conditions: %w", err)
	}
	if !conditionsValid {
		return false, nil
	}

	switch s.Ownership.PublicKey.Type {
	case signatures.PublicKey_KeyTypeSecp256k1:
		return crypto.VerifySignature(s.Ownership.PublicKey.PublicKey, hsh, s.Signature[:len(s.Signature)-1]), nil
	case signatures.PublicKey_KeyTypeBLSGroupSig:
		verKey := bls.BytesToVerKey(s.Ownership.PublicKey.PublicKey)
		verified, err := verKey.Verify(s.Signature, hsh)
		if err != nil {
			logger.Warningf("error verifying signature: %v", err)
			return false, xerrors.Errorf("error verifying: %w", err)
		}
		return verified, nil
	default:
		return false, xerrors.Errorf("Unknown key type %s", s.Ownership.PublicKey.Type)
	}
}

func validConditions(ctx context.Context, s *signatures.Signature, scope parens.Scope) (bool, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "signatures.validConditions")
	defer sp.Finish()

	if s.Ownership.Conditions == "" {
		return true, nil
	}

	if scope == nil {
		scope = parens.NewScope(defaultScope)
	}

	err := scope.Bind("hashed-preimage", func() string {
		return crypto.Keccak256Hash([]byte(s.PreImage)).String()
	})
	if err != nil {
		return false, xerrors.Errorf("error binding: %w", err)
	}

	res, err := parens.ExecuteStr(s.Ownership.Conditions, scope)
	if err != nil {
		return false, xerrors.Errorf("error executing script: %w", err)
	}
	if res == true {
		return true, nil
	}

	logger.Debugf("conditions for signature failed")
	return false, nil
}

func EcdsaToOwnership(key *ecdsa.PublicKey) *signatures.Ownership {
	return &signatures.Ownership{
		PublicKey: &signatures.PublicKey{
			Type:      signatures.PublicKey_KeyTypeSecp256k1,
			PublicKey: crypto.FromECDSAPub(key),
		},
	}
}

func BLSToOwnership(key *bls.VerKey) *signatures.Ownership {
	return &signatures.Ownership{
		PublicKey: &signatures.PublicKey{
			Type:      signatures.PublicKey_KeyTypeBLSGroupSig,
			PublicKey: key.Bytes(),
		},
	}
}

func SignerCount(sig *signatures.Signature) int {
	signerCount := 0
	for _, sigCount := range sig.Signers {
		if sigCount > 0 {
			signerCount++
		}
	}
	return signerCount
}

func BLSSign(ctx context.Context, key *bls.SignKey, hsh []byte, signerLen, signerIndex int) (*signatures.Signature, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "signatures.BLSSign")
	defer sp.Finish()

	if signerIndex >= signerLen {
		return nil, xerrors.Errorf("signer index must be less than signer length i: %d, l: %d", signerLen, signerIndex)
	}
	verKey, err := key.VerKey()
	if err != nil {
		return nil, xerrors.Errorf("error getting verkey: %w", err)
	}
	sig, err := key.Sign(hsh)
	if err != nil {
		return nil, xerrors.Errorf("Error signing: %w", err)
	}

	signers := make([]uint32, signerLen)
	signers[signerIndex] = 1
	return &signatures.Signature{
		Ownership: BLSToOwnership(verKey),
		Signers:   signers,
		Signature: sig,
	}, nil
}

func EcdsaSign(ctx context.Context, key *ecdsa.PrivateKey, hsh []byte) (*signatures.Signature, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "signatures.EcdsaSign")
	defer sp.Finish()

	sigBytes, err := crypto.Sign(hsh, key)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	return &signatures.Signature{
		Ownership: &signatures.Ownership{
			PublicKey: &signatures.PublicKey{
				Type: signatures.PublicKey_KeyTypeSecp256k1,
			},
		},
		Signature: sigBytes,
	}, nil
}

func AggregateBLSSignatures(ctx context.Context, sigs []*signatures.Signature) (*signatures.Signature, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "signatures.AggregateBLSSignatures")
	defer sp.Finish()

	signerCount := len(sigs[0].Signers)
	newSig := &signatures.Signature{
		Ownership: &signatures.Ownership{
			PublicKey: &signatures.PublicKey{
				Type: signatures.PublicKey_KeyTypeBLSGroupSig,
			},
		},
		Signers: make([]uint32, signerCount),
	}
	sigsToAggregate := make([][]byte, len(sigs))
	pubKeysToAggregate := make([]*bls.VerKey, len(sigs))
	for i, sig := range sigs {
		if sig.Ownership.PublicKey.Type != signatures.PublicKey_KeyTypeBLSGroupSig {
			return nil, xerrors.Errorf("wrong signature type, can only aggregate BLS signatures")
		}
		if len(sig.Signers) != signerCount {
			return nil, xerrors.Errorf("all signatures to aggregate must have the same signer length %d != %d", len(sig.Signers), signerCount)
		}
		sigsToAggregate[i] = sig.Signature
		pubKeysToAggregate[i] = bls.BytesToVerKey(sig.Ownership.PublicKey.PublicKey)
		for i, cnt := range sig.Signers {
			if existing := newSig.Signers[i]; cnt > math.MaxUint32-existing || existing > math.MaxUint32-cnt {
				return nil, xerrors.Errorf("error would overflow: %d %d", cnt, existing)
			}
			newSig.Signers[i] += cnt
		}
	}

	aggregateSig, err := bls.SumSignatures(sigsToAggregate)
	if err != nil {
		return nil, xerrors.Errorf("error summing signatures: %w", err)
	}
	newSig.Signature = aggregateSig

	aggregatePublic, err := bls.SumVerKeys(pubKeysToAggregate)
	if err != nil {
		return nil, xerrors.Errorf("error aggregating verKeys: %w", err)
	}
	newSig.Ownership.PublicKey.PublicKey = aggregatePublic.Bytes()
	return newSig, nil
}
