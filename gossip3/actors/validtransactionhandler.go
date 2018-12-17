package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

type conflictSetMap map[string]*conflictSet
type signatureMap map[string][]*messages.Signature
type transactionMap map[string]*messages.NewValidatedTransaction

type conflictSet struct {
	ID           string
	done         bool
	signatures   signatureMap
	didSign      bool
	transactions transactionMap
}

// returns true if one of the transactions has enough signatures
func (cs *conflictSet) possiblyDone(ng *types.NotaryGroup) *messages.NewValidatedTransaction {
	count := ng.QuorumCount()
	for tID, sigMap := range cs.signatures {
		if len(sigMap) >= count {
			return cs.transactions[tID]
		}
	}
	return nil
}

// TODO: detect deadlocks
func (cs *conflictSet) deadlocked(ng *types.NotaryGroup) bool {
	return false
}

type ValidTransactionHandler struct {
	middleware.LogAwareHolder

	self               *types.Signer
	notaryGroup        *types.NotaryGroup
	currentState       *actor.PID
	signatureGenerator *actor.PID
	doneCreator        *actor.PID
	signatureSender    *actor.PID
	conflictSets       conflictSetMap
}

type internalCreateDoneMessage struct {
	CurrentState *messages.CurrentState
	signatures   []*messages.Signature
}

func NewValidTransactionHandlerProps(currentState *actor.PID, self *types.Signer, group *types.NotaryGroup) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &ValidTransactionHandler{
			self:         self,
			notaryGroup:  group,
			currentState: currentState,
			conflictSets: make(conflictSetMap),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func newConflictSet(id string) *conflictSet {
	return &conflictSet{
		ID:           id,
		signatures:   make(signatureMap),
		transactions: make(transactionMap),
	}
}

func (vth *ValidTransactionHandler) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		vth.initialize(context)
	case *messages.Signature:
		vth.handleNewSignature(context, msg)
	case *messages.NewValidatedTransaction:
		vth.handleNewValidatedTransaction(context, msg)
	case *messages.CurrentState:
		vth.handleCurrentState(context, msg)
	}
}

// this function becomes an actor in its own right
func (vth *ValidTransactionHandler) signatureSenderActor(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.Signature:
		targets, err := vth.notaryGroup.RewardsCommittee([]byte(msg.ConflictSetID), vth.self)
		if err != nil {
			panic(fmt.Sprintf("error getting rewards committee: %v", err))
		}

		msg.Internal = false
		for _, t := range targets {
			vth.Log.Debugw("sending", "t", t.ID, "actor", t.Actor)
			t.Actor.Tell(msg)
		}
	}
}

func (vth *ValidTransactionHandler) createDoneMessageActor(context actor.Context) {
	switch msg := context.Message().(type) {
	case *internalCreateDoneMessage:
		mergedSigners := make([]bool, len(vth.notaryGroup.AllSigners()))
		var sigBytes [][]byte
		for _, sig := range msg.signatures {
			for i, ok := range sig.Signers {
				if ok {
					mergedSigners[i] = true
					sigBytes = append(sigBytes, sig.Signature)
				}
			}
		}
		signature, err := bls.SumSignatures(sigBytes)
		if err != nil {
			panic(fmt.Sprintf("error summing signatures: %v", err))
		}
		newSig := messages.Signature{
			Signers:   mergedSigners,
			Signature: signature,
		}
		currState := msg.CurrentState
		currState.Signature = newSig
		context.Respond(currState)
	}
}

// TODO: learned that initialize is probably not the right place to deo this stuff
func (vth *ValidTransactionHandler) initialize(context actor.Context) {
	sigGenerator, err := context.SpawnNamed(NewSignatureGeneratorProps(vth.self, vth.notaryGroup), "signatureGenerator")
	if err != nil {
		panic(fmt.Sprintf("error spawning sig generator: %v", err))
	}
	vth.signatureGenerator = sigGenerator

	signatureSender, err := context.SpawnNamed(actor.FromFunc(vth.signatureSenderActor), "signatureSender")
	if err != nil {
		panic(fmt.Sprintf("error spawning sig generator: %v", err))
	}
	vth.signatureSender = signatureSender

	doneCreator, err := context.SpawnNamed(actor.FromFunc(vth.createDoneMessageActor), "doneCreator")
	if err != nil {
		panic(fmt.Sprintf("error spawning sig generator: %v", err))
	}
	vth.doneCreator = doneCreator
}

func (vth *ValidTransactionHandler) handleNewValidatedTransaction(context actor.Context, msg *messages.NewValidatedTransaction) {
	cs := vth.getConflictSet(msg.ConflictSetID)
	if cs.done {
		// nothing to do here
		return
	}
	// TODO: handle the deadlock case
	if !cs.didSign {
		cs.didSign = true
		context.Request(vth.signatureGenerator, msg)
	}
	cs.transactions[string(msg.TransactionID)] = msg
}

func (vth *ValidTransactionHandler) handleCurrentState(context actor.Context, msg *messages.CurrentState) {
	cs := vth.getConflictSet(string(append(msg.ObjectID, msg.OldTip...)))
	vth.Log.Debugw("new current state")
	cs.done = true

	toRemove := make([][]byte, len(cs.transactions), len(cs.transactions))
	i := 0
	for k := range cs.transactions {
		toRemove[i] = []byte(k)
		i++
	}
	if len(toRemove) > 0 {
		context.Respond(&messages.MemPoolCleanup{
			Transactions: toRemove,
		})
	}
	cs.transactions = nil
	cs.signatures = nil

	vth.conflictSets[cs.ID] = cs
}

func (vth *ValidTransactionHandler) handleNewSignature(context actor.Context, msg *messages.Signature) {
	cs := vth.getConflictSet(msg.ConflictSetID)
	if cs.done {
		// nothing to do here
		return
	}
	if msg.Internal {
		vth.signatureSender.Tell(msg)
	}
	vth.Log.Debugw("new signature", "cs", cs.ID)

	sigs := cs.signatures[string(msg.TransactionID)]
	sigs = append(sigs, msg)
	cs.signatures[string(msg.TransactionID)] = sigs
	vth.conflictSets[cs.ID] = cs

	doneTrans := cs.possiblyDone(vth.notaryGroup)

	if doneTrans != nil {
		vth.doneCreator.Request(&internalCreateDoneMessage{
			CurrentState: &messages.CurrentState{
				ObjectID: doneTrans.ObjectID,
				Tip:      doneTrans.NewTip,
				OldTip:   doneTrans.OldTip,
			},
			signatures: cs.signatures[string(doneTrans.TransactionID)],
		}, context.Parent())
		return
	}
	if cs.deadlocked(vth.notaryGroup) {
		// handle deadlock
		return
	}
}

func (vth *ValidTransactionHandler) getConflictSet(id string) *conflictSet {
	cs, ok := vth.conflictSets[id]
	if !ok {
		cs = newConflictSet(id)
	}
	return cs
}
