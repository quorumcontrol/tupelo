package notary

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/ethereum/go-ethereum/crypto"
	"fmt"
	"context"
	"github.com/quorumcontrol/qc3/consensus"
	"log"
)

type ValidatorFunc func(ctx context.Context, chain *consensuspb.Chain, block *consensuspb.Block) (bool,error)

func sigFor(creator string, sigs []*consensuspb.Signature) (*consensuspb.Signature) {
	for _,sig := range sigs {
		if sig.Creator == creator {
			return sig
		}
	}
	return nil
}

func IsSigned(_ context.Context, chain *consensuspb.Chain, block *consensuspb.Block) (bool,error) {
	if len(block.Signatures) == 0 {
		log.Printf("block is not signed")
		return false, nil
	}
	return true,nil
}

func IsNotGenesisOrIsValidGenesis(_ context.Context, existingChain *consensuspb.Chain, block *consensuspb.Block) (bool,error) {
	log.Printf("existing chain: %v", existingChain)
	if block.SignableBlock == nil {
		log.Printf("no signable block")
		return false, nil
	}

	hsh,err := consensus.BlockToHash(block)
	if err != nil {
		return false, fmt.Errorf("error hashing block: %v", err)
	}

	// If this is a genesis block (a never before seen existingChain)
	if len(existingChain.Blocks) == 0 {
		log.Printf("this is a genesis block")
		// find the creator signature and validate that
		addr := consensus.DidToAddr(block.SignableBlock.ChainId)
		ownerSig := sigFor(addr, block.Signatures)
		if ownerSig == nil {
			return false, nil
		}

		ecdsaPubKey,err := crypto.SigToPub(hsh.Bytes(), ownerSig.Signature)
		if err != nil {
			return false, fmt.Errorf("error getting public key: %v", err)
		}
		if crypto.PubkeyToAddress(*ecdsaPubKey).Hex() != addr {
			return false, fmt.Errorf("unsigned by genesis address %s != %s", crypto.PubkeyToAddress(*ecdsaPubKey).Hex(), addr)
		}

		pubKey := &consensuspb.PublicKey{
			Type: consensuspb.Secp256k1,
			PublicKey: crypto.CompressPubkey(ecdsaPubKey),
			Id: addr,
		}

		return consensus.VerifySignature(block, pubKey, ownerSig)
	}
	log.Printf("returning true")
	return true, nil
}

func IsGenesisOrIsSignedByNecessaryOwners(ctx context.Context, existingChain *consensuspb.Chain, block *consensuspb.Block) (bool,error) {
	if len(existingChain.Blocks) == 0 {
		log.Printf("is a genesis block")
		return true,nil
	}

	authorizations := consensus.AuthorizationsByType(existingChain)
	updateAuth,ok := authorizations[consensuspb.UPDATE]
	var owners []*consensuspb.Chain
	minimum := uint64(1)

	if ok {
		log.Printf("found an authorization")
		owners = updateAuth.Owners
		minimum = updateAuth.Minimum
	} else {
		log.Printf("using existing chain")
		owners = []*consensuspb.Chain{existingChain}
	}

	signedByCount := uint64(0)
	for _,owner := range owners {
		log.Printf("detecting if signed by: %v", owner)
		signed,err := IsSignedBy(ctx, block, owner)
		if err != nil {
			return false, fmt.Errorf("error seeing if signed: %v", err)
		}
		if signed {
			signedByCount++
		}
	}

	if signedByCount >= minimum {
		return true, nil
	}

	return false, nil
}

func IsSignedBy(_ context.Context, block *consensuspb.Block, ownersChain *consensuspb.Chain) (bool,error) {
	ownersKeys := ownersChain.Authentication.PublicKeys

	sigs := consensus.SignaturesByCreator(block)
	log.Printf("sigs: %v", sigs)
	for _,key := range ownersKeys {
		sig,ok := sigs[key.Id]
		if ok {
			log.Printf("found signature for: %v", key.Id)
			verified,err := consensus.VerifySignature(block, key, sig)
			if err != nil {
				return false, fmt.Errorf("error verifying: %v", err)
			}
			if verified {
				return true, nil
			}
		} else {
			log.Printf("did not find signature")
		}
	}
	return false, nil
}