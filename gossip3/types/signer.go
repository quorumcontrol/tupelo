package types

import (
	"crypto/ecdsa"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/consensus"
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

func (s *Signer) ActorAddress() string {
	return hexutil.Encode(crypto.FromECDSAPub(s.DstKey))
}
