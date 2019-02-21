package types

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"sort"

	"github.com/AsynkronIT/protoactor-go/actor"
)

type NotaryGroup struct {
	ID        string
	Signers   map[string]*Signer
	sortedIds []string
}

func (ng *NotaryGroup) GetMajorityCount() int64 {
	required := int64(math.Ceil((2.0 * float64(len(ng.sortedIds))) / 3.0))
	if required == 0 {
		return 1
	}
	return required
}

func NewNotaryGroup(id string) *NotaryGroup {
	return &NotaryGroup{
		ID:      id,
		Signers: make(map[string]*Signer),
	}
}

func (ng *NotaryGroup) AddSigner(signer *Signer) {
	ng.Signers[signer.ID] = signer
	ng.sortedIds = append(ng.sortedIds, signer.ID)
	sort.Strings(ng.sortedIds)
}

func (ng *NotaryGroup) AllSigners() []*Signer {
	signers := make([]*Signer, len(ng.sortedIds), len(ng.sortedIds))
	for i, id := range ng.sortedIds {
		signers[i] = ng.Signers[id]
	}
	return signers
}

func (ng *NotaryGroup) SignerAtIndex(idx int) *Signer {
	id := ng.sortedIds[idx]
	return ng.Signers[id]
}

func (ng *NotaryGroup) IndexOfSigner(signer *Signer) uint64 {
	for i, s := range ng.sortedIds {
		if s == signer.ID {
			return uint64(i)
		}
	}
	panic(fmt.Sprintf("signer not found: %v", signer))
}

func (ng *NotaryGroup) Size() uint64 {
	return uint64(len(ng.sortedIds))
}

func (ng *NotaryGroup) QuorumCount() uint64 {
	return uint64(math.Ceil((float64(len(ng.sortedIds)) / 3) * 2))
}

func (ng *NotaryGroup) GetRandomSigner() *Signer {
	id := ng.sortedIds[randInt(len(ng.sortedIds))]
	return ng.Signers[id]
}

func (ng *NotaryGroup) GetRandomSyncer() *actor.PID {
	return ng.GetRandomSigner().Actor
}

func (ng *NotaryGroup) SetupAllRemoteActors(localKey *ecdsa.PublicKey) {
	for _, signer := range ng.AllSigners() {
		signer.Actor = actor.NewPID(signer.ActorAddress(localKey), "tupelo-"+signer.ID)
	}
}

func randInt(max int) int {
	bigInt, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic("bad random")
	}
	return int(bigInt.Int64())
}

const minSyncNodesPerTransaction = 3

func (ng *NotaryGroup) RewardsCommittee(key []byte, excluding *Signer) ([]*Signer, error) {
	signerCount := float64(len(ng.sortedIds))
	logOfSigners := math.Log(signerCount)
	numberOfTargets := math.Floor(math.Max(logOfSigners, float64(minSyncNodesPerTransaction)))
	indexSpacing := signerCount / numberOfTargets
	moduloOffset := math.Mod(float64(bytesToUint64(key)), indexSpacing)

	targets := make([]*Signer, 0, int(numberOfTargets))

	for i := 0; i < int(numberOfTargets); i++ {
		targetIndex := int64(math.Floor(moduloOffset + (indexSpacing * float64(i))))
		targetID := ng.sortedIds[targetIndex]
		target := ng.Signers[targetID]
		// Make sure this node doesn't add itself as a target
		if excluding.ID == target.ID {
			continue
		}
		targets = append(targets, target)

	}
	return targets, nil
}

func bytesToUint64(byteID []byte) uint64 {
	return binary.BigEndian.Uint64(byteID)
}
