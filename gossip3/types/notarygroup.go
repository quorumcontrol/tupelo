package types

import (
	"crypto/ecdsa"
	"math"
	"sort"

	"github.com/AsynkronIT/protoactor-go/actor"
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

func NewLocalSigner(dstKey *ecdsa.PublicKey, signKey *bls.SignKey) Signer {
	pubKey := consensus.BlsKeyToPublicKey(signKey.MustVerKey())
	return Signer{
		ID:      consensus.PublicKeyToAddr(&pubKey),
		SignKey: signKey,
		VerKey:  signKey.MustVerKey(),
		DstKey:  dstKey,
	}
}

func NewRemoteSigner(dstKey *ecdsa.PublicKey, verKey *bls.VerKey) Signer {
	pubKey := consensus.BlsKeyToPublicKey(verKey)
	return Signer{
		ID:     consensus.PublicKeyToAddr(&pubKey),
		VerKey: verKey,
		DstKey: dstKey,
	}
}

type NotaryGroup struct {
	Signers   map[string]Signer
	sortedIds []string
}

func (ng *NotaryGroup) GetMajorityCount() int64 {
	required := int64(math.Ceil((2.0 * float64(len(ng.sortedIds))) / 3.0))
	if required == 0 {
		return 1
	}
	return required
}

func NewNotaryGroup() *NotaryGroup {
	return &NotaryGroup{
		Signers: make(map[string]Signer),
	}
}

func (ng *NotaryGroup) AddSigner(signer Signer) {
	ng.Signers[signer.ID] = signer
	ng.sortedIds = append(ng.sortedIds, signer.ID)
	sort.Strings(ng.sortedIds)
}

func (ng *NotaryGroup) IndexOfSigner(signer Signer) int {
	for i, s := range ng.sortedIds {
		if s == signer.ID {
			return i
		}
	}
	return -1
}
