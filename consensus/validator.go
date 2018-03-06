package consensus

import (
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/ethereum/go-ethereum/crypto"
	"fmt"
)

func sigFor(creator string, sigs []*consensuspb.Signature) (*consensuspb.Signature) {
	for _,sig := range sigs {
		if sig.Creator == creator {
			return sig
		}
	}
	return nil
}

func IsValidGenesisBlock (chain *internalchain.InternalChain, block *consensuspb.Block) (bool,error) {
	if chain.LastBlock != nil || chain.Id != "" {
		return false, nil
	}

	if len(block.Signatures) == 0 {
		return false, nil
	}

	// find the creator signature and validate that
	addr := DidToAddr(block.SignableBlock.ChainId)
	ownerSig := sigFor(addr, block.Signatures)
	if ownerSig == nil {
		return false, nil
	}

	hsh,err := BlockToHash(block)
	if err != nil {
		return false, fmt.Errorf("error hashing block: %v", err)
	}

	pubKey,err := crypto.SigToPub(hsh.Bytes(), ownerSig.Signature)
	if err != nil {
		return false, fmt.Errorf("error getting public key: %v", err)
	}
	if crypto.PubkeyToAddress(*pubKey).Hex() != addr {
		return false, fmt.Errorf("unsigned by genesis address %s != %s", crypto.PubkeyToAddress(*pubKey).Hex(), addr)
	}

	return VerifySignature(block, &internalchain.InternalOwnership{
		PublicKeys: map[string]*consensuspb.PublicKey{
			addr: {
				PublicKey: crypto.CompressPubkey(pubKey),
			},
		},
	}, ownerSig)
}
