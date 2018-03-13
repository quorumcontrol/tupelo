package consensus

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"crypto/ecdsa"
	"github.com/gogo/protobuf/proto"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/bls"
)

func BlockToHash(block *consensuspb.Block) (hsh common.Hash, err error) {
	bytes,err := proto.Marshal(block.SignableBlock)
	if err != nil {
		return hsh, fmt.Errorf("error marshaling: %v", err)
	}

	return common.BytesToHash(bytes), nil
}

func AuthorizationsByType(chain *consensuspb.Chain) (map[consensuspb.Authorization_Type]*consensuspb.Authorization) {
	retMap := make(map[consensuspb.Authorization_Type]*consensuspb.Authorization)
	for _,auth := range chain.Authorizations {
		retMap[auth.Type] = auth
	}

	return retMap
}

func OwnerSignBlock(block *consensuspb.Block, key *ecdsa.PrivateKey) (*consensuspb.Block, error) {
	if block.SignableBlock == nil {
		return nil, fmt.Errorf("no signable block")
	}

	hsh,err := BlockToHash(block)
	if err != nil {
		return nil, fmt.Errorf("error hashing block: %v", err)
	}

	sigBytes,err := crypto.Sign(hsh.Bytes(), key)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	sig := &consensuspb.Signature{
		Creator: crypto.PubkeyToAddress(key.PublicKey).Hex(),
		Signature: sigBytes,
		Type: consensuspb.Secp256k1,
	}

	block.Signatures = append(block.Signatures, sig)

	return block,nil
}

func BlsSignBlock(block *consensuspb.Block, key *bls.SignKey) (*consensuspb.Block, error) {
	if block.SignableBlock == nil {
		return nil, fmt.Errorf("no signable block")
	}

	hsh,err := BlockToHash(block)
	if err != nil {
		return nil, fmt.Errorf("error hashing block: %v", err)
	}

	sigBytes,err := key.Sign(hsh.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	sig := &consensuspb.Signature{
		Creator: BlsVerKeyToAddress(key.MustVerKey().Bytes()).Hex(),
		Signature: sigBytes,
		Type: consensuspb.BLSGroupSig,
	}

	block.Signatures = append(block.Signatures, sig)
	return block, nil
}

func VerifySignature(block *consensuspb.Block, key *consensuspb.PublicKey, sig *consensuspb.Signature) (bool, error) {
	hsh,err := BlockToHash(block)
	if err != nil {
		return false, fmt.Errorf("error generating hash: %v", err)
	}

	switch sig.Type {
	case consensuspb.Secp256k1:
		return crypto.VerifySignature(key.PublicKey, hsh.Bytes(), sig.Signature[:len(sig.Signature)-1]), nil
	}

	return false, fmt.Errorf("unkown signature type")
}


func SignaturesByCreator(block *consensuspb.Block) (sigs map[string]*consensuspb.Signature) {
	sigs = make(map[string]*consensuspb.Signature)
	for _,sig := range block.Signatures {
		sigs[sig.Creator] = sig
	}
	return sigs
}
