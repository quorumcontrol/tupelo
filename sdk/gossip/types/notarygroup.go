package types

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sort"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/chaintree/chaintree"
)

// NotaryGroup represents a notary group.
type NotaryGroup struct {
	ID        string
	Signers   map[string]*Signer
	sortedIds []string
	config    *Config
}

func (ng *NotaryGroup) GetMajorityCount() int64 {
	required := int64(math.Ceil((2.0 * float64(len(ng.sortedIds))) / 3.0))
	if required == 0 {
		return 1
	}
	return required
}

// NewNotaryGroup instantiates a new NotaryGroup.
func NewNotaryGroup(id string) *NotaryGroup {
	c := DefaultConfig()
	c.ID = id
	return NewNotaryGroupFromConfig(c)
}

func NewNotaryGroupFromConfig(c *Config) *NotaryGroup {
	return &NotaryGroup{
		ID:      c.ID,
		Signers: make(map[string]*Signer),
		config:  c,
	}
}

func (ng *NotaryGroup) Config() *Config {
	return ng.config
}

func (ng *NotaryGroup) BlockValidators(ctx context.Context) ([]chaintree.BlockValidatorFunc, error) {
	return ng.config.blockValidators(ctx, ng)
}

// AddSigner adds a signer to group.
func (ng *NotaryGroup) AddSigner(signer *Signer) {
	ng.Signers[signer.ID] = signer
	ng.sortedIds = append(ng.sortedIds, signer.ID)
	sort.Strings(ng.sortedIds)
}

func (ng *NotaryGroup) AllSigners() []*Signer {
	signers := make([]*Signer, len(ng.sortedIds))
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
