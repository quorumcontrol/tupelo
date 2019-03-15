package actors

import (
	"fmt"
	"time"

	"github.com/quorumcontrol/storage"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/quorumcontrol/tupelo-go-client/bls"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

type signaturesBySigner map[string]*messages.SignatureWrapper
type signaturesByTransaction map[string]signaturesBySigner
type transactionMap map[string]*messages.TransactionWrapper

type checkStateMsg struct {
	atUpdate uint64
}

type ConflictSet struct {
	middleware.LogAwareHolder

	ID                 string
	currentStateStore  storage.Reader
	notaryGroup        *types.NotaryGroup
	signatureGenerator *actor.PID
	signatureChecker   *actor.PID
	signatureSender    *actor.PID
	signer             *types.Signer

	done          bool
	signatures    signaturesByTransaction
	signerSigs    signaturesBySigner
	didSign       bool
	transactions  transactionMap
	snoozedCommit *messages.CurrentStateWrapper
	view          uint64
	updates       uint64
	active        bool

	behavior actor.Behavior
}

type ConflictSetConfig struct {
	ID                 string
	NotaryGroup        *types.NotaryGroup
	Signer             *types.Signer
	SignatureGenerator *actor.PID
	SignatureChecker   *actor.PID
	SignatureSender    *actor.PID
	CurrentStateStore  storage.Reader
}

func NewConflictSetProps(cfg *ConflictSetConfig) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		c := &ConflictSet{
			ID:                 cfg.ID,
			currentStateStore:  cfg.CurrentStateStore,
			notaryGroup:        cfg.NotaryGroup,
			signer:             cfg.Signer,
			signatureGenerator: cfg.SignatureGenerator,
			signatureChecker:   cfg.SignatureChecker,
			signatureSender:    cfg.SignatureSender,
			signatures:         make(signaturesByTransaction),
			signerSigs:         make(signaturesBySigner),
			transactions:       make(transactionMap),
			behavior:           actor.NewBehavior(),
		}

		c.behavior.Become(c.NormalState)

		return c
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (cs *ConflictSet) Receive(ctx actor.Context) {
	cs.behavior.Receive(ctx)
}

func (cs *ConflictSet) NormalState(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.TransactionWrapper:
		cs.handleNewTransaction(context, msg)
	// this will be an external signature
	case *extmsgs.Signature:
		wrapper, err := sigToWrapper(msg, cs.notaryGroup, cs.signer, false)
		if err != nil {
			panic(fmt.Sprintf("error wrapping sig: %v", err))
		}
		cs.handleNewSignature(context, wrapper)
	case *messages.SignatureWrapper:
		cs.handleNewSignature(context, msg)
	case *extmsgs.CurrentState:
		cs.Log.Errorw("something called this")
	case *commitNotification:
		err := cs.handleCommit(context, msg)
		if err != nil {
			panic(err)
		}
	case *checkStateMsg:
		cs.checkState(context, msg)
	case *messages.ActivateSnoozingConflictSets:
		cs.activate(context, msg)
	}
}

func (cs *ConflictSet) activate(context actor.Context, msg *messages.ActivateSnoozingConflictSets) {
	cs.Log.Debug("activate")

	cs.active = true

	var err error
	if cs.snoozedCommit != nil {
		err = cs.handleCurrentStateWrapper(context, cs.snoozedCommit)
	}
	if err != nil {
		panic(fmt.Errorf("error processing snoozed commit: %v", err))
	}

	if cs.done {
		// We had a valid commit already, so we're done
		return
	}

	// no (valid) commit, so let's start validating any snoozed transactions
	if parent := context.Parent(); parent != nil {
		for _, transaction := range cs.transactions {
			context.Request(parent, &messages.ValidateTransaction{
				Key:   transaction.Key,
				Value: transaction.Value,
			})
		}
	}
}

func (cs *ConflictSet) handleCommit(context actor.Context, msg *commitNotification) error {
	cs.Log.Debug("handleCommit")

	var currState extmsgs.CurrentState
	_, err := currState.UnmarshalMsg(msg.store.Value)
	if err != nil {
		panic(fmt.Errorf("error unmarshaling: %v", err))
	}
	wrapper := &messages.CurrentStateWrapper{
		CurrentState: &currState,
		Internal:     false,
		Key:          msg.store.Key,
		Value:        msg.store.Value,
		Metadata:     messages.MetadataMap{"seen": time.Now()},
	}

	if cs.active {
		return cs.handleCurrentStateWrapper(context, wrapper)
	} else {
		verified, err := cs.validSignature(context, wrapper)
		if err != nil {
			panic(fmt.Errorf("error verifying signature: %v", err))
		}
		if verified {
			wrapper.Verified = true
			wrapper.Metadata["verifiedAt"] = time.Now()
			if msg.height == msg.nextHeight {
				cs.active = true
				return cs.handleCurrentStateWrapper(context, wrapper)
			}

			cs.Log.Debug("snoozing commit for later")
			if cs.snoozedCommit == nil {
				cs.snoozedCommit = wrapper
			} else {
				panic(fmt.Errorf("received new commit with one already snoozed"))
			}
			return nil
		}
		return fmt.Errorf("signature not verified")
	}
}

func (cs *ConflictSet) DoneReceive(context actor.Context) {
	cs.Log.Debugw("done cs received message")
	// do nothing when in the done state
}

func (cs *ConflictSet) handleNewTransaction(context actor.Context, msg *messages.TransactionWrapper) {
	sp := msg.NewSpan("conflictset-handlenewtransaction")
	defer sp.Finish()

	cs.Log.Debugw("new transaction", "trans", msg.TransactionID)
	if !msg.PreFlight && !msg.Accepted {
		panic(fmt.Sprintf("we should only handle pre-flight or accepted transactions at this level"))
	}

	if msg.Accepted {
		sp.LogKV("accepted", msg.Accepted)
		cs.active = true
	}

	if !cs.active {
		sp.LogKV("snoozing", true)
		cs.Log.Debugw("snoozing transaction", "t", msg.Key, "height", msg.Transaction.Height)
	}
	cs.transactions[string(msg.TransactionID)] = msg
	if cs.active {
		cs.processTransactions(context)
	}
}

func (cs *ConflictSet) processTransactions(context actor.Context) {
	if !cs.active {
		panic(fmt.Errorf("error: processTransactions called on inactive ConflictSet"))
	}

	for _, transaction := range cs.transactions {
		sp := transaction.NewSpan("conflictset-processing")
		cs.Log.Debugw("processing transaction", "t", transaction.Key, "height", transaction.Transaction.Height)

		if !cs.didSign {
			context.Request(cs.signatureGenerator, transaction)
			sp.LogKV("didSign", true)
			cs.didSign = true
		}
		cs.updates++
		sp.Finish()
	}
	// do this as a message to make sure we're doing it after all the updates have come in
	context.Send(context.Self(), &checkStateMsg{atUpdate: cs.updates})
}

func (cs *ConflictSet) handleNewSignature(context actor.Context, msg *messages.SignatureWrapper) {
	cs.Log.Debugw("handle new signature", "t", msg.Signature.TransactionID)
	if msg.Internal {
		context.Send(cs.signatureSender, msg)
	}
	if len(msg.Signers) > 1 {
		panic(fmt.Sprintf("currently we don't handle multi signer signatures here"))
	}
	existingMap, ok := cs.signatures[string(msg.Signature.TransactionID)]
	if !ok {
		existingMap = make(signaturesBySigner)
		for id := range msg.Signers {
			existingMap[id] = msg
		}
	} else {
		for id := range msg.Signers {
			_, ok := existingMap[id]
			if ok {
				// we already have this sig
				return
			}
			existingMap[id] = msg
		}
	}
	cs.signatures[string(msg.Signature.TransactionID)] = existingMap
	for id := range msg.Signers {
		//Note (TB): this is probably a good place to look for slashable offenses
		cs.signerSigs[id] = msg
	}

	cs.updates++
	context.Send(context.Self(), &checkStateMsg{atUpdate: cs.updates})
}

func (cs *ConflictSet) checkState(context actor.Context, msg *checkStateMsg) {
	cs.Log.Debugw("check state")
	if cs.updates < msg.atUpdate {
		cs.Log.Debugw("old update")
		// we know there will be another check state message with a higher update
		return
	}
	if trans := cs.possiblyDone(); trans != nil {
		sp := trans.NewSpan("checkState")
		defer sp.Finish()
		// we have a possibly done transaction, lets make a current state
		if err := cs.createCurrentStateFromTrans(context, trans); err != nil {
			panic(err)
		}
		return
	}

	if cs.deadlocked() {
		cs.handleDeadlockedState(context)
	}
}

func (cs *ConflictSet) handleDeadlockedState(context actor.Context) {
	cs.Log.Debugw("handle deadlocked state")

	var lowestTrans *messages.TransactionWrapper
	for transID, trans := range cs.transactions {
		trans.LogKV("deadlocked", true)
		if lowestTrans == nil {
			lowestTrans = trans
			continue
		}
		if transID < string(lowestTrans.TransactionID) {
			lowestTrans = trans
		}
	}
	cs.nextView(lowestTrans)

	cs.handleNewTransaction(context, lowestTrans)
}

func (cs *ConflictSet) nextView(newWinner *messages.TransactionWrapper) {
	cs.view++
	cs.didSign = false
	cs.transactions = make(transactionMap)

	// only keep signatures on the winning transaction
	transSigs := cs.signatures[string(newWinner.TransactionID)]
	cs.signatures = signaturesByTransaction{string(newWinner.TransactionID): transSigs}
	cs.signerSigs = transSigs
}

func (cs *ConflictSet) createCurrentStateFromTrans(context actor.Context, trans *messages.TransactionWrapper) error {
	sp := trans.NewSpan("createCurrentState")
	defer sp.Finish()

	cs.Log.Debugw("createCurrentStateFromTrans", "t", trans.Key)
	sigs := cs.signatures[string(trans.TransactionID)]
	var sigBytes [][]byte
	signersArray := bitarray.NewSparseBitArray()
	for _, sig := range sigs {
		other, err := bitarray.Unmarshal(sig.Signature.Signers)
		if err != nil {
			return fmt.Errorf("error unmarshaling: %v", err)
		}
		signersArray = signersArray.Or(other)
		sigBytes = append(sigBytes, sig.Signature.Signature)
	}

	summed, err := bls.SumSignatures(sigBytes)
	if err != nil {
		return fmt.Errorf("error summing signatures: %v", err)
	}

	marshaled, err := bitarray.Marshal(signersArray)
	if err != nil {
		return fmt.Errorf("error marshaling bitarray: %v", err)
	}

	currState := &extmsgs.CurrentState{
		Signature: &extmsgs.Signature{
			TransactionID: trans.TransactionID,
			ObjectID:      trans.Transaction.ObjectID,
			PreviousTip:   trans.Transaction.PreviousTip,
			Height:        trans.Transaction.Height,
			NewTip:        trans.Transaction.NewTip,
			Signers:       marshaled,
			Signature:     summed,
		},
	}

	marshaledState, err := currState.MarshalMsg(nil)
	if err != nil {
		return fmt.Errorf("error marshaling: %v", err)
	}

	currStateWrapper := &messages.CurrentStateWrapper{
		Internal:     true,
		CurrentState: currState,
		Key:          currState.CommittedKey(),
		Value:        marshaledState,
		Metadata:     messages.MetadataMap{"seen": time.Now()},
	}

	// don't use message passing, because we can skip a lot of processing if we're done right here
	return cs.handleCurrentStateWrapper(context, currStateWrapper)
}

func (cs *ConflictSet) validSignature(context actor.Context, currWrapper *messages.CurrentStateWrapper) (bool, error) {
	sig := currWrapper.CurrentState.Signature
	signerArray, err := bitarray.Unmarshal(sig.Signers)
	if err != nil {
		return false, fmt.Errorf("error unmarshaling: %v", err)
	}
	var verKeys [][]byte

	signers := cs.notaryGroup.AllSigners()
	for i, signer := range signers {
		isSet, err := signerArray.GetBit(uint64(i))
		if err != nil {
			return false, fmt.Errorf("error getting bit: %v", err)
		}
		if isSet {
			verKeys = append(verKeys, signer.VerKey.Bytes())
		}
	}

	cs.Log.Debugw("checking signature", "len", len(verKeys))
	resp, err := context.RequestFuture(cs.signatureChecker, &messages.SignatureVerification{
		Message:   sig.GetSignable(),
		Signature: sig.Signature,
		VerKeys:   verKeys,
	}, 10*time.Second).Result()

	if err != nil {
		return false, fmt.Errorf("error waiting for signature validation: %v", err)
	}

	return resp.(*messages.SignatureVerification).Verified, nil
}

func (cs *ConflictSet) handleCurrentStateWrapper(context actor.Context, currWrapper *messages.CurrentStateWrapper) error {
	cs.Log.Debugw("handleCurrentStateWrapper", "internal", currWrapper.Internal)

	if !currWrapper.Verified {
		verified, err := cs.validSignature(context, currWrapper)
		if err != nil {
			panic(fmt.Errorf("error verifying signature: %v", err))
		}
		if verified {
			currWrapper.Verified = true
			currWrapper.Metadata["verifiedAt"] = time.Now()
		}
	}

	if currWrapper.Verified {
		currWrapper.CleanupTransactions = make([]*messages.TransactionWrapper, len(cs.transactions))
		i := 0
		for _, t := range cs.transactions {
			t.LogKV("done", true)
			currWrapper.CleanupTransactions[i] = t
			i++
		}

		cs.done = true
		cs.Log.Debugw("done")
		cs.behavior.Become(cs.DoneReceive)

		if parent := context.Parent(); parent != nil {
			context.Send(parent, currWrapper)
		}
	} else {
		cs.Log.Errorw("signature not verified")
	}
	return nil
}

// returns a transaction with enough signatures or nil if none yet exist
func (cs *ConflictSet) possiblyDone() *messages.TransactionWrapper {
	count := cs.notaryGroup.QuorumCount()
	for tID, sigList := range cs.signatures {
		cs.Log.Debugw("check count", "t", []byte(tID), "len", len(sigList), "quorumAt", count)
		if uint64(len(sigList)) >= count {
			return cs.transactions[tID]
		}
	}
	return nil
}

func (cs *ConflictSet) deadlocked() bool {
	unknownSigCount := len(cs.notaryGroup.Signers) - len(cs.signerSigs)
	quorumAt := cs.notaryGroup.QuorumCount()
	if len(cs.signerSigs) == 0 {
		return false
	}
	for _, sigs := range cs.signatures {
		if uint64(len(sigs)+unknownSigCount) >= quorumAt {
			return false
		}
	}

	return true
}

func sigToWrapper(sig *extmsgs.Signature, ng *types.NotaryGroup, self *types.Signer, isInternal bool) (*messages.SignatureWrapper, error) {
	signerMap := make(messages.SignerMap)
	signerBitMap, err := bitarray.Unmarshal(sig.Signers)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling bit array: %v", err)
	}
	allSigners := ng.AllSigners()
	for i, signer := range allSigners {
		isSet, err := signerBitMap.GetBit(uint64(i))
		if err != nil {
			return nil, fmt.Errorf("error getting bit: %v", err)
		}
		if isSet {
			signerMap[signer.ID] = signer
		}
	}

	conflictSetID := extmsgs.ConflictSetID(sig.ObjectID, sig.Height)

	committee, err := ng.RewardsCommittee([]byte(sig.NewTip), self)
	if err != nil {
		return nil, fmt.Errorf("error getting committee: %v", err)
	}

	return &messages.SignatureWrapper{
		Internal:         isInternal,
		ConflictSetID:    conflictSetID,
		RewardsCommittee: committee,
		Signers:          signerMap,
		Signature:        sig,
		Metadata:         messages.MetadataMap{"seen": time.Now()},
	}, nil
}
