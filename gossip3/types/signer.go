package types

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip3/remote"
	"github.com/quorumcontrol/tupelo/p2p"
)

type Signer struct {
	ID     string
	DstKey *ecdsa.PublicKey
	VerKey *bls.VerKey

	SignKey *bls.SignKey
	Actor   *actor.PID
}

func NewLocalSigner(dstKey *ecdsa.PublicKey, signKey *bls.SignKey) *Signer {
	pubKey := consensus.BlsKeyToPublicKey(signKey.MustVerKey())
	return &Signer{
		ID:      consensus.PublicKeyToAddr(&pubKey),
		SignKey: signKey,
		VerKey:  signKey.MustVerKey(),
		DstKey:  dstKey,
	}
}

func NewRemoteSigner(dstKey *ecdsa.PublicKey, verKey *bls.VerKey) *Signer {
	pubKey := consensus.BlsKeyToPublicKey(verKey)
	return &Signer{
		ID:     consensus.PublicKeyToAddr(&pubKey),
		VerKey: verKey,
		DstKey: dstKey,
	}
}

func (s *Signer) ActorAddress(fromSigner *Signer) string {
	fromID, err := p2p.PeerFromEcdsaKey(fromSigner.DstKey)
	if err != nil {
		panic(fmt.Sprintf("error getting peer from ecdsa key: %v", err))
	}
	id, err := p2p.PeerFromEcdsaKey(s.DstKey)
	if err != nil {
		panic(fmt.Sprintf("error getting peer from ecdsa key: %v", err))
	}
	return remote.NewRoutableAddress(fromID.Pretty(), id.Pretty()).String()
}
