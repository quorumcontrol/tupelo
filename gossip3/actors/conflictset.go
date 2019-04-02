package actors

import (
	"fmt"
	"reflect"
	"time"

	"github.com/quorumcontrol/storage"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/quorumcontrol/tupelo-go-client/bls"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-client/tracing"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

type signaturesBySigner map[string]*messages.SignatureWrapper
type signaturesByTransaction map[string]signaturesBySigner
type transactionMap map[string]*messages.TransactionWrapper

type checkStateMsg struct {
	atUpdate uint64
}

type csWorkerRequest struct {
	msg interface{}
	cs  *ConflictSet
}

// implements the necessary interface for consistent hashing
// router
func (cswr *csWorkerRequest) Hash() string {
	return cswr.cs.ID
}

type ConflictSet struct {
	tracing.ContextHolder

	ID string

	done          bool
	signatures    signaturesByTransaction
	signerSigs    signaturesBySigner
	didSign       bool
	transactions  transactionMap
	snoozedCommit *messages.CurrentStateWrapper
	view          uint64
	updates       uint64
	active        bool

	height     uint64
	nextHeight uint64
}

type ConflictSetConfig struct {
	NotaryGroup        *types.NotaryGroup
	Signer             *types.Signer
	SignatureGenerator *actor.PID
	SignatureChecker   *actor.PID
	SignatureSender    *actor.PID
	CurrentStateStore  storage.Reader
	ConflictSetRouter  *actor.PID
}

func NewConflictSet(id string) *ConflictSet {
	c := &ConflictSet{
		ID:           id,
		signatures:   make(signaturesByTransaction),
		signerSigs:   make(signaturesBySigner),
		transactions: make(transactionMap),
	}
	return c
}

const conflictSetConcurrency = 50

func NewConflictSetWorkerPool(cfg *ConflictSetConfig) *actor.Props {
	// it's important that this is a consistent hash pool rather than round robin
	// because we do not want two operations on a single conflictset executing concurrently
	// if you change this, make sure you provide some other "locking" mechanism.
	return router.NewConsistentHashPool(conflictSetConcurrency).WithProducer(func() actor.Actor {
		return &ConflictSetWorker{
			router:             cfg.ConflictSetRouter,
			currentStateStore:  cfg.CurrentStateStore,
			notaryGroup:        cfg.NotaryGroup,
			signer:             cfg.Signer,
			signatureGenerator: cfg.SignatureGenerator,
			signatureChecker:   cfg.SignatureChecker,
			signatureSender:    cfg.SignatureSender,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

type ConflictSetWorker struct {
	middleware.LogAwareHolder
	tracing.ContextHolder

	router             *actor.PID
	currentStateStore  storage.Reader
	notaryGroup        *types.NotaryGroup
	signatureGenerator *actor.PID
	signatureChecker   *actor.PID
	signatureSender    *actor.PID
	signer             *types.Signer
}

func NewConflictSetWorkerProps(csr *actor.PID) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &ConflictSetWorker{router: csr}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (csw *ConflictSetWorker) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started, *actor.Stopping, *actor.Stopped:
		// do nothing
	case *csWorkerRequest:
		if msg.cs.done {
			csw.Log.Debugw("received message on done CS")
			return
		}
		csw.OriginalReceive(msg.cs, msg.msg, context)
	default:
		csw.Log.Errorw("received bad message", "type", reflect.TypeOf(context.Message()).String())
	}
}

func (csw *ConflictSetWorker) OriginalReceive(cs *ConflictSet, sentMsg interface{}, context actor.Context) {
	switch msg := sentMsg.(type) {
	case *messages.TransactionWrapper:
		csw.handleNewTransaction(cs, context, msg)
	// this will be an external signature
	case *extmsgs.Signature:
		wrapper, err := sigToWrapper(msg, csw.notaryGroup, csw.signer, false)
		if err != nil {
			panic(fmt.Sprintf("error wrapping sig: %v", err))
		}
		csw.handleNewSignature(cs, context, wrapper)
	case *messages.SignatureWrapper:
		csw.handleNewSignature(cs, context, msg)
	case *messages.CurrentStateWrapper:
		csw.handleCurrentStateWrapper(cs, context, msg)
	case *extmsgs.CurrentState:
		csw.Log.Errorw("something called this")
	case *commitNotification:
		err := csw.handleCommit(cs, context, msg)
		if err != nil {
			panic(err)
		}
	case *checkStateMsg:
		csw.checkState(cs, context, msg)
	case *messages.ActivateSnoozingConflictSets:
		csw.activate(cs, context, msg)
	}
}

func (csw *ConflictSetWorker) activate(cs *ConflictSet, context actor.Context, msg *messages.ActivateSnoozingConflictSets) {
	sp := cs.NewSpan("activate")
	defer sp.Finish()
	csw.Log.Debug("activate")

	cs.active = true

	var err error
	if cs.snoozedCommit != nil {
		err = csw.handleCurrentStateWrapper(cs, context, cs.snoozedCommit)
	}
	if err != nil {
		panic(fmt.Errorf("error processing snoozed commit: %v", err))
	}

	if cs.done {
		// We had a valid commit already, so we're done
		return
	}

	// no (valid) commit, so let's start validating any snoozed transactions
	for _, transaction := range cs.transactions {
		context.Send(csw.router, &messages.ValidateTransaction{
			Key:   transaction.Key,
			Value: transaction.Value,
		})
	}
}

func (csw *ConflictSetWorker) handleCommit(cs *ConflictSet, context actor.Context, msg *commitNotification) error {
	sp := cs.NewSpan("handleCommit")
	defer sp.Finish()

	csw.Log.Debug("handleCommit")

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

	if msg.height == msg.nextHeight {
		sp.SetTag("activating", true)
		cs.active = true
	}

	return csw.validSignature(cs, context, wrapper)
}

func (csw *ConflictSetWorker) handleNewTransaction(cs *ConflictSet, context actor.Context, msg *messages.TransactionWrapper) {
	sp := cs.NewSpan("handleNewTransaction")
	defer sp.Finish()
	sp.SetTag("transaction", msg.TransactionID)

	transSpan := msg.NewSpan("conflictset-handlenewtransaction")
	defer transSpan.Finish()

	csw.Log.Debugw("new transaction", "trans", msg.TransactionID)
	if !msg.PreFlight && !msg.Accepted {
		panic(fmt.Sprintf("we should only handle pre-flight or accepted transactions at this level"))
	}

	if msg.Accepted {
		sp.SetTag("accepted", true)
		sp.SetTag("active", true)
		transSpan.SetTag("accepted", true)
		cs.active = true
	}

	if !cs.active {
		sp.SetTag("snoozing", true)
		transSpan.SetTag("snoozing", true)
		csw.Log.Debugw("snoozing transaction", "t", msg.Key, "height", msg.Transaction.Height)
	}
	cs.transactions[string(msg.TransactionID)] = msg
	if cs.active {
		csw.processTransactions(cs, context)
	}
}

func (csw *ConflictSetWorker) processTransactions(cs *ConflictSet, context actor.Context) {
	sp := cs.NewSpan("processTransactions")
	defer sp.Finish()

	if !cs.active {
		panic(fmt.Errorf("error: processTransactions called on inactive ConflictSet"))
	}

	for _, transaction := range cs.transactions {
		transSpan := transaction.NewSpan("conflictset-processing")
		csw.Log.Debugw("processing transaction", "t", transaction.Key, "height", transaction.Transaction.Height)

		if !cs.didSign {
			context.RequestWithCustomSender(csw.signatureGenerator, transaction, csw.router)
			transSpan.SetTag("didSign", true)
			cs.didSign = true
		}
		cs.updates++
		transSpan.Finish()
	}
	// do this as a message to make sure we're doing it after all the updates have come in
	context.Send(context.Self(), &csWorkerRequest{cs: cs, msg: &checkStateMsg{atUpdate: cs.updates}})
}

func (csw *ConflictSetWorker) handleNewSignature(cs *ConflictSet, context actor.Context, msg *messages.SignatureWrapper) {
	sp := cs.NewSpan("handleNewSignature")
	defer sp.Finish()

	csw.Log.Debugw("handle new signature", "t", msg.Signature.TransactionID)
	if msg.Internal {
		context.Send(csw.signatureSender, msg)
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
	csw.Log.Debugw("sending checkstate", "self", context.Self().String())
	context.Send(context.Self(), &csWorkerRequest{cs: cs, msg: &checkStateMsg{atUpdate: cs.updates}})
}

func (csw *ConflictSetWorker) checkState(cs *ConflictSet, context actor.Context, msg *checkStateMsg) {
	sp := cs.NewSpan("checkState")
	defer sp.Finish()

	csw.Log.Debugw("check state")
	if cs.updates < msg.atUpdate {
		csw.Log.Debugw("old update")
		sp.SetTag("oldUpdate", true)
		// we know there will be another check state message with a higher update
		return
	}
	if trans := csw.possiblyDone(cs); trans != nil {
		transSpan := trans.NewSpan("checkState")
		defer transSpan.Finish()
		// we have a possibly done transaction, lets make a current state
		if err := csw.createCurrentStateFromTrans(cs, context, trans); err != nil {
			panic(err)
		}
		return
	}

	if csw.deadlocked(cs) {
		csw.handleDeadlockedState(cs, context)
	}
}

func (csw *ConflictSetWorker) handleDeadlockedState(cs *ConflictSet, context actor.Context) {
	sp := cs.NewSpan("handleDeadlockedState")
	defer sp.Finish()
	csw.Log.Debugw("handle deadlocked state")

	var lowestTrans *messages.TransactionWrapper
	for transID, trans := range cs.transactions {
		transSpan := trans.NewSpan("handleDeadlockedState")
		defer transSpan.Finish()
		if lowestTrans == nil {
			lowestTrans = trans
			continue
		}
		if transID < string(lowestTrans.TransactionID) {
			lowestTrans = trans
		}
	}
	cs.nextView(lowestTrans)

	csw.handleNewTransaction(cs, context, lowestTrans)
}

func (cs *ConflictSet) nextView(newWinner *messages.TransactionWrapper) {
	sp := cs.NewSpan("nextView")
	defer sp.Finish()

	cs.view++
	cs.didSign = false
	cs.transactions = make(transactionMap)

	// only keep signatures on the winning transaction
	transSigs := cs.signatures[string(newWinner.TransactionID)]
	cs.signatures = signaturesByTransaction{string(newWinner.TransactionID): transSigs}
	cs.signerSigs = transSigs
}

func (csw *ConflictSetWorker) createCurrentStateFromTrans(cs *ConflictSet, context actor.Context, trans *messages.TransactionWrapper) error {
	sp := cs.NewSpan("createCurrentStateFromTrans")
	defer sp.Finish()
	transSpan := trans.NewSpan("createCurrentState")
	defer transSpan.Finish()

	sp.SetTag("winner", trans.TransactionID)

	csw.Log.Debugw("createCurrentStateFromTrans", "t", trans.Key)
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
	return csw.validSignature(cs, context, currStateWrapper)
}

func (csw *ConflictSetWorker) validSignature(cs *ConflictSet, context actor.Context, currWrapper *messages.CurrentStateWrapper) error {
	sp := cs.NewSpan("validSignature")
	defer sp.Finish()

	sig := currWrapper.CurrentState.Signature
	signerArray, err := bitarray.Unmarshal(sig.Signers)
	if err != nil {
		return fmt.Errorf("error unmarshaling: %v", err)
	}
	var verKeys [][]byte

	signers := csw.notaryGroup.AllSigners()
	for i, signer := range signers {
		isSet, err := signerArray.GetBit(uint64(i))
		if err != nil {
			return fmt.Errorf("error getting bit: %v", err)
		}
		if isSet {
			verKeys = append(verKeys, signer.VerKey.Bytes())
		}
	}

	csw.Log.Debugw("checking signature", "len", len(verKeys))
	context.RequestWithCustomSender(csw.signatureChecker, &messages.SignatureVerification{
		Message:   sig.GetSignable(),
		Signature: sig.Signature,
		VerKeys:   verKeys,
		Memo:      currWrapper,
	}, csw.router)

	return nil
}

func (csw *ConflictSetWorker) handleCurrentStateWrapper(cs *ConflictSet, context actor.Context, currWrapper *messages.CurrentStateWrapper) error {
	sp := cs.NewSpan("handleCurrentStateWrapper")
	defer sp.Finish()

	csw.Log.Debugw("handleCurrentStateWrapper", "internal", currWrapper.Internal)

	if currWrapper.Verified {
		if !cs.active {
			if cs.snoozedCommit != nil {
				return fmt.Errorf("received new commit with one already snoozed")
			}
			cs.snoozedCommit = currWrapper
			return nil
		}
		currWrapper.CleanupTransactions = make([]*messages.TransactionWrapper, len(cs.transactions))
		i := 0
		for _, t := range cs.transactions {
			transSpan := t.NewSpan("handleCurrentStateWrapper")
			defer transSpan.Finish()
			transSpan.SetTag("done", true)
			currWrapper.CleanupTransactions[i] = t
			i++
		}

		cs.done = true
		sp.SetTag("done", true)
		csw.Log.Debugw("done")

		context.Send(csw.router, currWrapper)
		return nil
	}

	sp.SetTag("badSignature", true)
	sp.SetTag("error", true)
	csw.Log.Errorw("signature not verified AND SHOULD NEVER GET HERE")
	return nil
}

// returns a transaction with enough signatures or nil if none yet exist
func (csw *ConflictSetWorker) possiblyDone(cs *ConflictSet) *messages.TransactionWrapper {
	sp := cs.NewSpan("possiblyDone")
	defer sp.Finish()

	count := csw.notaryGroup.QuorumCount()
	for tID, sigList := range cs.signatures {
		if uint64(len(sigList)) >= count {
			return cs.transactions[tID]
		}
	}
	return nil
}

func (csw *ConflictSetWorker) deadlocked(cs *ConflictSet) bool {
	sp := cs.NewSpan("isDeadlocked")
	defer sp.Finish()

	unknownSigCount := len(csw.notaryGroup.Signers) - len(cs.signerSigs)
	quorumAt := csw.notaryGroup.QuorumCount()
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
