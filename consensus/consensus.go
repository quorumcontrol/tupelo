package consensus

import (
	"fmt"
	"strings"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/ethereum/go-ethereum/common"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/ethereum/go-ethereum/log"
)

func init() {
	typecaster.AddType(StandardHeaders{})
	typecaster.AddType(Signature{})
	typecaster.AddType(PublicKey{})
}

const (
	KeyTypeBLSGroupSig = "BLS"
	KeyTypeSecp256k1 = "secp256k1"
)

type PublicKey struct {
	Id string `refmt:"id,omitempty" json:"id,omitempty" cbor:"id,omitempty"`
	Type string `refmt:"type,omitempty" json:"type,omitempty" cbor:"type,omitempty"`
	PublicKey []byte `refmt:"publicKey,omitempty" json:"publicKey,omitempty" cbor:"publicKey,omitempty"`
}

type Signature struct {
	Signers []bool `refmt:"signers,omitempty" json:"signers,omitempty" cbor:"signers,omitempty"`
	Signature []byte `refmt:"signature,omitempty" json:"signature,omitempty" cbor:"signature,omitempty"`
	Type string `refmt:"type,omitempty" json:"type,omitempty" cbor:"type,omitempty"`
}

type StandardHeaders struct {
	Signatures map[string]Signature `refmt:"signatures,omitempty" json:"signatures,omitempty" cbor:"signatures,omitempty"`
}

func AddrToDid (addr string) string {
	return fmt.Sprintf("did:qc:%s", addr)
}

func DidToAddr(did string) string {
	segs := strings.Split(did, ":")
	return segs[len(segs) - 1]
}

func BlsVerKeyToAddress(pubBytes []byte) common.Address {
	return common.BytesToAddress(crypto.Keccak256(pubBytes)[12:])
}

func EcdsaToPublicKey(key *ecdsa.PublicKey) (PublicKey) {
	return PublicKey{
		Type: KeyTypeSecp256k1,
		PublicKey: crypto.CompressPubkey(key),
		Id: crypto.PubkeyToAddress(*key).Hex(),
	}
}

func BlsKeyToPublicKey(key *bls.VerKey) (PublicKey) {
	return PublicKey{
		Id: BlsVerKeyToAddress(key.Bytes()).Hex(),
		PublicKey: key.Bytes(),
		Type: KeyTypeBLSGroupSig,
	}
}

func BlockToHash(block chaintree.Block) ([]byte, error) {
	sw := &dag.SafeWrap{}

	hsh := sw.WrapObject(block).Cid().Hash()
	if sw.Err != nil {
		return nil, fmt.Errorf("error wrapping block")
	}

	return hsh[2:],nil
}

func NewEmptyTree(did string) *dag.BidirectionalTree {
	sw := &dag.SafeWrap{}
	treeNode := sw.WrapObject(make(map[string]string))

	chainNode := sw.WrapObject(make(map[string]string))

	root := sw.WrapObject(map[string]interface{}{
		"chain": chainNode.Cid(),
		"tree": treeNode.Cid(),
		"id": did,
	})

	// sanity check
	if sw.Err != nil {
		panic(sw.Err)
	}

	return dag.NewBidirectionalTree(root.Cid(), root, treeNode, chainNode)
}


func SignBlock(blockWithHeaders *chaintree.BlockWithHeaders, key *ecdsa.PrivateKey) (*chaintree.BlockWithHeaders, error) {
	hsh,err := BlockToHash(blockWithHeaders.Block)
	if err != nil {
		return nil, fmt.Errorf("error hashing block: %v", err)
	}

	sigBytes,err := crypto.Sign(hsh, key)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	addr := crypto.PubkeyToAddress(key.PublicKey).String()

	sig := Signature{
		Signature: sigBytes,
		Type: KeyTypeSecp256k1,
	}

	headers := &StandardHeaders{}
	err = typecaster.ToType(blockWithHeaders.Headers, headers)
	if err != nil {
		return nil, fmt.Errorf("error casting headers: %v", err)
	}


	if headers.Signatures == nil {
		headers.Signatures = make(map[string]Signature)
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

	return blockWithHeaders,nil
}



func IsBlockSignedBy(blockWithHeaders *chaintree.BlockWithHeaders, addr string) (bool, error) {
	headers := &StandardHeaders{}
	if blockWithHeaders.Headers != nil {
		err := typecaster.ToType(blockWithHeaders.Headers, headers)
		if err != nil {
			return false, fmt.Errorf("error converting headers: %v", err)
		}
	}

	sig,ok := headers.Signatures[addr]
	if !ok {
		log.Error("no signature", "signatures", headers.Signatures)
		return false, nil
	}


	hsh,err := BlockToHash(blockWithHeaders.Block)
	if err != nil {
		log.Error("error wrapping block")
		return false, fmt.Errorf("error wrapping block: %v", err)
	}

	switch sig.Type {
	case KeyTypeSecp256k1:
		log.Debug("sig to pub")
		ecdsaPubKey,err := crypto.SigToPub(hsh, sig.Signature)
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
