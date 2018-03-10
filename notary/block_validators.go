package notary

import (
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/ethereum/go-ethereum/crypto"
	"fmt"
	"context"
	"log"
	"github.com/quorumcontrol/qc3/consensus"
)

type ValidatorFunc func(ctx context.Context, chain *internalchain.InternalChain, block *consensuspb.Block) (bool,error)

func sigFor(creator string, sigs []*consensuspb.Signature) (*consensuspb.Signature) {
	for _,sig := range sigs {
		if sig.Creator == creator {
			return sig
		}
	}
	return nil
}

// IsValidOwnerSig verifies that the current block is signed by the correct owner.
// In the case of a genesis block, it must be signed by the key matching the did address
// otherwise,
func IsValidOwnerSig (_ context.Context, chain *internalchain.InternalChain, block *consensuspb.Block) (bool,error) {
	if len(block.Signatures) == 0 {
		return false, nil
	}

	hsh,err := consensus.BlockToHash(block)
	if err != nil {
		return false, fmt.Errorf("error hashing block: %v", err)
	}

	// If this is a genesis block (a never before seen chain)
	if chain.LastBlock == nil {
		// find the creator signature and validate that
		addr := consensus.DidToAddr(block.SignableBlock.ChainId)
		ownerSig := sigFor(addr, block.Signatures)
		if ownerSig == nil {
			return false, nil
		}

		pubKey,err := crypto.SigToPub(hsh.Bytes(), ownerSig.Signature)
		if err != nil {
			return false, fmt.Errorf("error getting public key: %v", err)
		}
		if crypto.PubkeyToAddress(*pubKey).Hex() != addr {
			return false, fmt.Errorf("unsigned by genesis address %s != %s", crypto.PubkeyToAddress(*pubKey).Hex(), addr)
		}

		return consensus.VerifySignature(block, &internalchain.InternalOwnership{
			PublicKeys: map[string]*consensuspb.PublicKey{
				addr: {
					PublicKey: crypto.CompressPubkey(pubKey),
				},
			},
		}, ownerSig)
	} else {
		// we have seen this chain before, let's see who the current owners are.
		signedCount := 0
		sigs := consensus.SignaturesByCreator(block)
		for _,owner := range chain.CurrentOwners {
			for _,publicKey := range owner.PublicKeys {
				sig, ok := sigs[publicKey.Id]
				if ok {
					verified,err := consensus.VerifySignature(block, owner, sig)
					if err == nil && verified {
						signedCount++
					} else {
						log.Printf("error verifying: %v", err)
					}
				}
			}
		}
		if signedCount >= chain.MinimumOwners {
			return true, nil
		}
	}

	return false, nil
}

