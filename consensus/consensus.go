package consensus

import (
	"crypto/ecdsa"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/qc3/bls"
)

func init() {
	typecaster.AddType(StandardHeaders{})
	typecaster.AddType(Signature{})
	typecaster.AddType(PublicKey{})
	cbornode.RegisterCborType(StandardHeaders{})
	cbornode.RegisterCborType(Signature{})
	cbornode.RegisterCborType(PublicKey{})
}

type SignatureMap map[string]Signature

const (
	KeyTypeBLSGroupSig = "BLS"
	KeyTypeSecp256k1   = "secp256k1"
)

type PublicKey struct {
	Id        string `refmt:"id,omitempty" json:"id,omitempty" cbor:"id,omitempty"`
	Type      string `refmt:"type,omitempty" json:"type,omitempty" cbor:"type,omitempty"`
	PublicKey []byte `refmt:"publicKey,omitempty" json:"publicKey,omitempty" cbor:"publicKey,omitempty"`
}

type Signature struct {
	Signers   []bool `refmt:"signers,omitempty" json:"signers,omitempty" cbor:"signers,omitempty"`
	Signature []byte `refmt:"signature,omitempty" json:"signature,omitempty" cbor:"signature,omitempty"`
	Type      string `refmt:"type,omitempty" json:"type,omitempty" cbor:"type,omitempty"`
}

type StandardHeaders struct {
	Signatures SignatureMap `refmt:"signatures,omitempty" json:"signatures,omitempty" cbor:"signatures,omitempty"`
}

func AddrToDid(addr string) string {
	return fmt.Sprintf("did:qc:%s", addr)
}

func DidToAddr(did string) string {
	segs := strings.Split(did, ":")
	return segs[len(segs)-1]
}

// ToEcdsaPub returns the ecdsa typed key from the bytes in the PublicKey
// at this time there is no error checking.
func (pk *PublicKey) ToEcdsaPub() *ecdsa.PublicKey {
	return crypto.ToECDSAPub(pk.PublicKey)
}

func PublicKeyToAddr(key *PublicKey) string {
	switch key.Type {
	case KeyTypeSecp256k1:
		return crypto.PubkeyToAddress(*crypto.ToECDSAPub(key.PublicKey)).String()
	case KeyTypeBLSGroupSig:
		return BlsVerKeyToAddress(key.PublicKey).String()
	default:
		return ""
	}
}

func BlsVerKeyToAddress(pubBytes []byte) common.Address {
	return common.BytesToAddress(crypto.Keccak256(pubBytes)[12:])
}

func EcdsaToPublicKey(key *ecdsa.PublicKey) PublicKey {
	return PublicKey{
		Type:      KeyTypeSecp256k1,
		PublicKey: crypto.FromECDSAPub(key),
		Id:        crypto.PubkeyToAddress(*key).String(),
	}
}

func BlsKeyToPublicKey(key *bls.VerKey) PublicKey {
	return PublicKey{
		Id:        BlsVerKeyToAddress(key.Bytes()).Hex(),
		PublicKey: key.Bytes(),
		Type:      KeyTypeBLSGroupSig,
	}
}

func BlockToHash(block chaintree.Block) ([]byte, error) {
	return ObjToHash(block)
}

func NewEmptyTree(did string) *dag.BidirectionalTree {
	sw := &dag.SafeWrap{}
	treeNode := sw.WrapObject(make(map[string]string))

	chainNode := sw.WrapObject(make(map[string]string))

	root := sw.WrapObject(map[string]interface{}{
		"chain": chainNode.Cid(),
		"tree":  treeNode.Cid(),
		"id":    did,
	})

	// sanity check
	if sw.Err != nil {
		panic(sw.Err)
	}

	return dag.NewBidirectionalTree(root.Cid(), root, treeNode, chainNode)
}

func SignBlock(blockWithHeaders *chaintree.BlockWithHeaders, key *ecdsa.PrivateKey) (*chaintree.BlockWithHeaders, error) {
	hsh, err := BlockToHash(blockWithHeaders.Block)
	if err != nil {
		return nil, fmt.Errorf("error hashing block: %v", err)
	}

	sigBytes, err := crypto.Sign(hsh, key)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	addr := crypto.PubkeyToAddress(key.PublicKey).String()

	sig := Signature{
		Signature: sigBytes,
		Type:      KeyTypeSecp256k1,
	}

	headers := &StandardHeaders{}
	err = typecaster.ToType(blockWithHeaders.Headers, headers)
	if err != nil {
		return nil, fmt.Errorf("error casting headers: %v", err)
	}

	if headers.Signatures == nil {
		headers.Signatures = make(SignatureMap)
	}

	headers.Signatures[addr] = sig

	var marshaledHeaders map[string]interface{}
	err = typecaster.ToType(headers, &marshaledHeaders)
	if err != nil {
		return nil, fmt.Errorf("error casting headers: %v", err)
	}

	blockWithHeaders.Headers = marshaledHeaders

	if err != nil {
		return nil, fmt.Errorf("error getting jsonish")
	}

	return blockWithHeaders, nil
}

func IsBlockSignedBy(blockWithHeaders *chaintree.BlockWithHeaders, addr string) (bool, error) {
	headers := &StandardHeaders{}
	if blockWithHeaders.Headers != nil {
		err := typecaster.ToType(blockWithHeaders.Headers, headers)
		if err != nil {
			return false, fmt.Errorf("error converting headers: %v", err)
		}
	}

	sig, ok := headers.Signatures[addr]
	if !ok {
		log.Error("no signature", "signatures", headers.Signatures)
		return false, nil
	}

	hsh, err := BlockToHash(blockWithHeaders.Block)
	if err != nil {
		log.Error("error wrapping block")
		return false, fmt.Errorf("error wrapping block: %v", err)
	}

	switch sig.Type {
	case KeyTypeSecp256k1:
		log.Debug("sig to pub")
		ecdsaPubKey, err := crypto.SigToPub(hsh, sig.Signature)
		if err != nil {
			return false, fmt.Errorf("error getting public key: %v", err)
		}
		if crypto.PubkeyToAddress(*ecdsaPubKey).String() != addr {
			return false, fmt.Errorf("unsigned by genesis address %s != %s", crypto.PubkeyToAddress(*ecdsaPubKey).Hex(), addr)
		}

		log.Debug("testing signature")
		return crypto.VerifySignature(crypto.FromECDSAPub(ecdsaPubKey), hsh, sig.Signature[:len(sig.Signature)-1]), nil
	}

	log.Error("unknown signature type")
	return false, fmt.Errorf("unkown signature type")
}

func BlsSign(payload interface{}, key *bls.SignKey) (*Signature, error) {
	hsh, err := ObjToHash(payload)
	if err != nil {
		return nil, fmt.Errorf("error hashing block: %v", err)
	}

	sigBytes, err := key.Sign(hsh)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	sig := &Signature{
		Signature: sigBytes,
		Type:      KeyTypeBLSGroupSig,
	}

	return sig, nil
}

func EcdsaSign(payload interface{}, key *ecdsa.PrivateKey) (*Signature, error) {
	hsh, err := ObjToHash(payload)
	if err != nil {
		return nil, fmt.Errorf("error hashing block: %v", err)
	}

	sigBytes, err := crypto.Sign(hsh, key)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}
	return &Signature{
		Signature: sigBytes,
		Type:      KeyTypeSecp256k1,
	}, nil
}

func Verify(hsh []byte, sig Signature, key PublicKey) (bool, error) {
	switch sig.Type {
	case KeyTypeSecp256k1:
		recoverdPub, err := crypto.SigToPub(hsh, sig.Signature)
		if err != nil {
			return false, fmt.Errorf("error recovering signature: %v", err)
		}

		if crypto.PubkeyToAddress(*recoverdPub).String() != PublicKeyToAddr(&key) {
			return false, nil
		}

		return crypto.VerifySignature(crypto.FromECDSAPub(recoverdPub), hsh, sig.Signature[:len(sig.Signature)-1]), nil
	case KeyTypeBLSGroupSig:
		verKey := bls.BytesToVerKey(key.PublicKey)
		verified, err := verKey.Verify(sig.Signature, hsh)
		if err != nil {
			log.Error("error verifying", "err", err)
			return false, fmt.Errorf("error verifying: %v", err)
		}
		return verified, nil
	default:
		log.Error("unknown signature type", "type", sig.Type)
		return false, fmt.Errorf("error: unknown signature type: %v", sig.Type)
	}
}

func ObjToHash(payload interface{}) ([]byte, error) {
	sw := &dag.SafeWrap{}

	wrapped := sw.WrapObject(payload)
	if sw.Err != nil {
		return nil, fmt.Errorf("error wrapping block: %v", sw.Err)
	}

	hsh := wrapped.Cid().Hash()

	return hsh[2:], nil
}

func MustObjToHash(payload interface{}) []byte {
	hsh, err := ObjToHash(payload)
	if err != nil {
		panic(fmt.Sprintf("error hashing %v", payload))
	}
	return hsh
}
