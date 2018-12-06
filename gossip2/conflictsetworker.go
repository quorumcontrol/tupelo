package gossip2

import (
	"bytes"
	"context"
	"fmt"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
)

type conflictSetWorker struct {
	gn *GossipNode
}

func (csw *conflictSetWorker) HandleRequest(msg ProvideMessage, respCh chan processorResponse) {
	gn := csw.gn
	messageType := MessageType(msg.Key[8])
	conflictSetID := conflictSetIDFromMessageKey(msg.Key)

	opts := []opentracing.StartSpanOption{opentracing.Tag{Key: "span.kind", Value: "consumer"}}
	if spanContext := msg.spanContext; spanContext != nil {
		opts = append(opts, opentracing.FollowsFrom(spanContext))
	}
	span, ctx := newSpan(context.Background(), gn.Tracer, "ConflictSetWorker", opts...)
	defer span.Finish()

	log.Debugf("%s conflict set worker %s", gn.ID(), msg.Key)
	switch messageType {
	case MessageTypeSignature:
		log.Debugf("%v: handling a new Signature message", gn.ID())
		_, err := csw.HandleNewSignature(ctx, msg)
		queueSpan, _ := newSpan(ctx, gn.Tracer, "QueueProcessorResponse")
		if err != nil {
			respCh <- processorResponse{
				ConflictSetID: conflictSetID,
				Error:         err,
				spanContext:   queueSpan.Context(),
			}
			return
		}
		respCh <- processorResponse{
			ConflictSetID: conflictSetID,
			NewSignature:  true,
			spanContext:   queueSpan.Context(),
		}
		queueSpan.Finish()

	case MessageTypeTransaction:
		log.Debugf("%v: handling a new Transaction message", gn.ID())
		didSign, err := csw.HandleNewTransaction(ctx, msg)
		queueSpan, _ := newSpan(ctx, gn.Tracer, "QueueProcessorResponse")
		if err != nil {
			respCh <- processorResponse{
				ConflictSetID: conflictSetID,
				Error:         err,
				spanContext:   queueSpan.Context(),
			}
			return
		}
		respCh <- processorResponse{
			ConflictSetID:  conflictSetID,
			NewTransaction: didSign,
			spanContext:    queueSpan.Context(),
		}
		queueSpan.Finish()

	case MessageTypeDone:
		log.Debugf("%v: handling a new Done message", gn.ID())
		err := csw.handleDone(ctx, msg)
		queueSpan, _ := newSpan(ctx, gn.Tracer, "QueueProcessorResponse")
		if err != nil {
			respCh <- processorResponse{
				ConflictSetID: conflictSetID,
				Error:         err,
				spanContext:   queueSpan.Context(),
			}
			return
		}
		respCh <- processorResponse{
			ConflictSetID: conflictSetID,
			IsDone:        true,
			spanContext:   queueSpan.Context(),
		}
		queueSpan.Finish()
	default:
		log.Errorf("%v: unknown message %v", gn.ID(), msg.Key)
	}
}

func (csw *conflictSetWorker) handleDone(ctx context.Context, msg ProvideMessage) error {
	gn := csw.gn
	log.Debugf("%s handling done message %v", gn.ID(), msg.Key)

	span, _ := newSpan(ctx, gn.Tracer, "HandleDone")
	defer span.Finish()

	var state CurrentState
	_, err := state.UnmarshalMsg(msg.Value)
	if err != nil {
		return fmt.Errorf("error unmarshaling msg: %v", err)
	}
	verified, err := state.Verify(gn.Group)
	if (err != nil) || !verified {
		if !verified {
			gn.Remove(msg.Key)
		}
		return fmt.Errorf("invalid done message: %v", err)
	}

	err = gn.Storage.Set(objectIdWithPrefix(state.ObjectID), msg.Value)
	if err != nil {
		return fmt.Errorf("error setting current state: %v", err)
	}

	go gn.subscriptions.Notify(state.ObjectID, state)

	conflictSetID := conflictSetIDFromMessageKey(msg.Key)
	conflictSetKeys, err := gn.Storage.GetKeysByPrefix(conflictSetID[0:4])
	if err != nil {
		return err
	}
	for _, key := range conflictSetKeys {
		switch messageTypeFromKey(key) {
		case MessageTypeDone:
			continue
		case MessageTypeSignature:
			if bytes.Equal(conflictSetIDFromMessageKey(key), conflictSetID) {
				gn.Storage.Delete(key)
			}
		default:
			if bytes.Equal(conflictSetIDFromMessageKey(key), conflictSetID) {
				gn.Remove(key)
			}
		}
	}

	return nil
}

func (csw *conflictSetWorker) HandleNewSignature(ctx context.Context, msg ProvideMessage) (accepted bool, err error) {
	span, _ := newSpan(ctx, csw.gn.Tracer, "HandleNewSignature")
	defer span.Finish()

	log.Debugf("%s handling sig message %v", csw.gn.ID(), msg.Key)
	// TOOD: actually check to see if we accept this sig (although, only maybe... we can check it when we actually care, that is when we are summing up)
	return true, nil
}

func (csw *conflictSetWorker) HandleNewTransaction(ctx context.Context, msg ProvideMessage) (didSign bool, err error) {
	gn := csw.gn
	//TODO: check if transaction hash matches key hash
	//TODO: check if the conflict set is done
	//TODO: sign this transaction if it's valid and new
	span, funcCtx := newSpan(ctx, gn.Tracer, "HandleNewTransaction")
	defer span.Finish()

	var t Transaction
	_, err = t.UnmarshalMsg(msg.Value)

	if err != nil {
		return false, fmt.Errorf("error getting transaction: %v", err)
	}
	log.Debugf("%s new transaction %s", gn.ID(), bytesToString(t.ID()))

	isValid, err := csw.IsTransactionValid(funcCtx, t)
	if err != nil {
		return false, fmt.Errorf("error validating transaction: %v", err)
	}

	if isValid {
		roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
		if err != nil {
			return false, fmt.Errorf("error getting round info: %v", err)
		}
		sig, err := t.Sign(gn.SignKey)
		if err != nil {
			return false, fmt.Errorf("error signing key: %v", err)
		}
		signers := make([]bool, len(roundInfo.Signers))
		signers[indexOfAddr(roundInfo.Signers, gn.address)] = true
		signature := Signature{
			TransactionID: t.ID(),
			ObjectID:      t.ObjectID,
			Tip:           t.NewTip,
			Signers:       signers,
			Signature:     sig,
		}
		encodedSig, err := signature.MarshalMsg(nil)
		if err != nil {
			return false, fmt.Errorf("error marshaling sig: %v", err)
		}
		sigID := signature.StoredID(t.ToConflictSet().ID())
		log.Debugf("%s signing transaction %v, signature id %v", gn.ID(), msg.Key, sigID)

		sigMessage := ProvideMessage{
			Key:   sigID,
			Value: encodedSig,
			From:  gn.ID(),
		}

		go func(aMsg ProvideMessage) {
			gn.newObjCh <- aMsg
		}(sigMessage)

		targets, err := gn.syncTargetsByRoutingKey(t.NewTip)
		if err != nil {
			log.Errorf("%s error getting routes: %v", gn.ID(), err)
		}

		for _, target := range targets {
			if target != nil {
				gn.sigSendingCh <- envelope{
					Msg:         sigMessage,
					Destination: *(target.DstKey.ToEcdsaPub()),
				}
			}
		}

		return true, nil
	}

	gn.Remove(msg.Key)
	log.Errorf("%s error, invalid transaction", gn.ID())
	return false, nil
}

func (csw *conflictSetWorker) IsTransactionValid(ctx context.Context, t Transaction) (bool, error) {
	gn := csw.gn

	span, _ := newSpan(ctx, gn.Tracer, "IsTransactionValid")
	defer span.Finish()

	state, err := gn.getCurrentState(t.ObjectID)
	if err != nil {
		return false, fmt.Errorf("error getting current state: %v", err)
	}
	var currentTip []byte
	if state != nil {
		currentTip = state.Tip
	}

	stateTransition := StateTransaction{
		CurrentState: currentTip,
		Transaction:  t.Payload,
		ObjectID:     t.ObjectID,
	}

	newState, isAccepted, err := chainTreeStateHandler(context.TODO(), stateTransition)
	if err != nil || !isAccepted {
		return false, err
	}

	if !bytes.Equal(newState, t.NewTip) {
		log.Errorf("%s tips did not match: %v : %v", gn.ID(), newState, t.NewTip)
		return false, nil
	}

	// here we would send transaction and state to a handler
	return true, nil
}
