package consensus

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"

	"golang.org/x/crypto/scrypt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/messages/v2/build/go/signatures"
	sigfuncs "github.com/quorumcontrol/tupelo/sdk/signatures"
)

func init() {
	typecaster.AddType(StandardHeaders{})
	cbornode.RegisterCborType(StandardHeaders{})
	typecaster.AddType(signatures.TreeState{})
	cbornode.RegisterCborType(signatures.TreeState{})
}

type SignatureMap map[string]*signatures.Signature

// Merge returns a new SignatatureMap composed of the original with the other merged in
// other wins when both SignatureMaps have signatures
func (sm SignatureMap) Merge(other SignatureMap) SignatureMap {
	newSm := make(SignatureMap)
	for k, v := range sm {
		newSm[k] = v
	}
	for k, v := range other {
		newSm[k] = v
	}
	return newSm
}

// Subtract returns a new SignatureMap with only the new signatures
// in other.
func (sm SignatureMap) Subtract(other SignatureMap) SignatureMap {
	newSm := make(SignatureMap)
	for k, v := range sm {
		_, ok := other[k]
		if !ok {
			newSm[k] = v
		}
	}
	return newSm
}

// Only returns a new SignatureMap with only keys in the keys
// slice argument
func (sm SignatureMap) Only(keys []string) SignatureMap {
	newSm := make(SignatureMap)
	for _, key := range keys {
		v, ok := sm[key]
		if ok {
			newSm[key] = v
		}
	}
	return newSm
}

type StandardHeaders struct {
	Signatures SignatureMap `refmt:"signatures,omitempty" json:"signatures,omitempty" cbor:"signatures,omitempty"`
}

func AddrToDid(addr string) string {
	return fmt.Sprintf("did:tupelo:%s", addr)
}

func EcdsaPubkeyToDid(key ecdsa.PublicKey) string {
	keyAddr := crypto.PubkeyToAddress(key).String()

	return AddrToDid(keyAddr)
}

func DidToAddr(did string) string {
	segs := strings.Split(did, ":")
	return segs[len(segs)-1]
}

func BlockToHash(block chaintree.Block) ([]byte, error) {
	return ObjToHash(block)
}

func NewEmptyTree(ctx context.Context, did string, nodeStore nodestore.DagStore) *dag.Dag {
	sw := &safewrap.SafeWrap{}
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
	dag, err := dag.NewDagWithNodes(ctx, nodeStore, root, treeNode, chainNode)
	if err != nil {
		panic(err) // TODO: this err was introduced, keeping external interface the same
	}
	return dag
}

func SignBlock(ctx context.Context, blockWithHeaders *chaintree.BlockWithHeaders, key *ecdsa.PrivateKey) (*chaintree.BlockWithHeaders, error) {
	hsh, err := BlockToHash(blockWithHeaders.Block)
	if err != nil {
		return nil, fmt.Errorf("error hashing block: %v", err)
	}

	sigBytes, err := crypto.Sign(hsh, key)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	addr := crypto.PubkeyToAddress(key.PublicKey).String()

	sig := signatures.Signature{
		Ownership: &signatures.Ownership{
			PublicKey: &signatures.PublicKey{
				Type: signatures.PublicKey_KeyTypeSecp256k1,
			},
		},
		Signature: sigBytes,
	}

	headers := &StandardHeaders{}
	err = typecaster.ToType(blockWithHeaders.Headers, headers)
	if err != nil {
		return nil, fmt.Errorf("error casting headers: %v", err)
	}

	if headers.Signatures == nil {
		headers.Signatures = make(SignatureMap)
	}

	headers.Signatures[addr] = &sig

	var marshaledHeaders map[string]interface{}
	err = typecaster.ToType(headers, &marshaledHeaders)
	if err != nil {
		return nil, fmt.Errorf("error casting headers: %v", err)
	}

	blockWithHeaders.Headers = marshaledHeaders

	return blockWithHeaders, nil
}

func IsBlockSignedBy(ctx context.Context, blockWithHeaders *chaintree.BlockWithHeaders, addr string) (bool, error) {
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

	switch sig.Ownership.PublicKey.Type {
	case signatures.PublicKey_KeyTypeSecp256k1:
		err := sigfuncs.RestoreEcdsaPublicKey(ctx, sig, hsh)
		if err != nil {
			return false, fmt.Errorf("error restoring public key: %v", err)
		}
		sigAddr, err := sigfuncs.Address(sig.Ownership)
		if err != nil {
			return false, fmt.Errorf("error getting address from signature: %v", err)
		}
		if sigAddr.String() != addr {
			return false, fmt.Errorf("unsigned by address %s != %s", sigAddr.String(), addr)
		}
		return sigfuncs.Valid(ctx, sig, hsh, nil) // TODO: maybe we want to have a custom scope here?
	}

	log.Error("unknown signature type")
	return false, fmt.Errorf("unkown signature type")
}

func ObjToHash(payload interface{}) ([]byte, error) {
	sw := &safewrap.SafeWrap{}

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

// PassPhraseKey implements a known passphrase -> private Key generator
// following very closely the params from Warp Wallet.
// The only difference here is that the N on scrypt is 256 instead of 218 because
// go requires N to be a power of 2.
// from the Warp Wallet ( https://keybase.io/warp/warp_1.0.9_SHA256_a2067491ab582bde779f4505055807c2479354633a2216b22cf1e92d1a6e4a87.html ):
// s1	=	scrypt(key=(passphrase||0x1), salt=(salt||0x1), N=218, r=8, p=1, dkLen=32)
// s2	=	pbkdf2(key=(passphrase||0x2), salt=(salt||0x2), c=216, dkLen=32, prf=HMAC_SHA256)
// keypair	=	generate_bitcoin_keypair(s1 ⊕ s2)
func PassPhraseKey(passPhrase, salt []byte) (*ecdsa.PrivateKey, error) {
	if len(passPhrase) == 0 {
		return nil, fmt.Errorf("error, must specify a passPhrase")
	}
	var firstSalt, secondSalt []byte
	if len(salt) == 0 {
		firstSalt = []byte{1}
		secondSalt = []byte{2}
	} else {
		hashedSalt := sha256.Sum256(salt)
		firstSalt = hashedSalt[:]
		secondSalt = hashedSalt[:]
	}

	s1, err := scrypt.Key(passPhrase, firstSalt, 256, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("error running scyrpt: %v", err)
	}
	s2 := pbkdf2.Key(passPhrase, secondSalt, 216, 32, sha256.New)
	dst := make([]byte, 32)
	safeXORBytes(dst, s1, s2, 32)
	return crypto.ToECDSA(dst)
}

// n needs to be smaller or equal than the length of a and b.
func safeXORBytes(dst, a, b []byte, n int) {
	for i := 0; i < n; i++ {
		dst[i] = a[i] ^ b[i]
	}
}
