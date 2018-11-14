package gossip2

import (
	"bytes"
	"context"
	"fmt"
	"time"
)

type conflictSetWorker struct {
	gn *GossipNode
}

func (csw *conflictSetWorker) HandleRequest(msg ProvideMessage, respCh chan processorResponse) {
	gn := csw.gn
	messageType := MessageType(msg.Key[8])
	conflictSetID := conflictSetIDFromMessageKey(msg.Key)

	log.Debugf("%s conflict set worker %s", gn.ID(), msg.Key)
	switch messageType {
	case MessageTypeSignature:
		log.Debugf("%v: handling a new Signature message", gn.ID())
		_, err := csw.HandleNewSignature(msg)
		if err != nil {
			respCh <- processorResponse{
				ConflictSetID: conflictSetID,
				Error:         err,
			}
		}
		respCh <- processorResponse{
			ConflictSetID: conflictSetID,
			NewSignature:  true,
		}
	case MessageTypeTransaction:
		log.Debugf("%v: handling a new Transaction message", gn.ID())
		didSign, err := csw.HandleNewTransaction(msg)
		if err != nil {
			respCh <- processorResponse{
				ConflictSetID: conflictSetID,
				Error:         err,
			}
		}
		respCh <- processorResponse{
			ConflictSetID:  conflictSetID,
			NewTransaction: didSign,
		}
	case MessageTypeDone:
		log.Debugf("%v: handling a new Done message", gn.ID())
		err := csw.handleDone(msg)
		if err != nil {
			respCh <- processorResponse{
				ConflictSetID: conflictSetID,
				Error:         err,
			}
		}
		respCh <- processorResponse{
			ConflictSetID: conflictSetID,
			IsDone:        true,
		}
	default:
		log.Errorf("%v: unknown message %v", gn.ID(), msg.Key)
	}
}

func (csw *conflictSetWorker) handleDone(msg ProvideMessage) error {
	gn := csw.gn
	log.Debugf("%s handling done message %v", gn.ID(), msg.Key)

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

	err = gn.Storage.Set(state.ObjectID, msg.Value)
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

func (csw *conflictSetWorker) HandleNewSignature(msg ProvideMessage) (accepted bool, err error) {
	log.Debugf("%s handling sig message %v", csw.gn.ID(), msg.Key)
	// TOOD: actually check to see if we accept this sig (although, only maybe... we can check it when we actually care, that is when we are summing up)
	return true, nil
}

func (csw *conflictSetWorker) HandleNewTransaction(msg ProvideMessage) (didSign bool, err error) {
	gn := csw.gn
	//TODO: check if transaction hash matches key hash
	//TODO: check if the conflict set is done
	//TODO: sign this transaction if it's valid and new

	var t Transaction
	_, err = t.UnmarshalMsg(msg.Value)

	if err != nil {
		return false, fmt.Errorf("error getting transaction: %v", err)
	}
	log.Debugf("%s new transaction %s", gn.ID(), bytesToString(t.ID()))

	isValid, err := csw.IsTransactionValid(t)
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

func (csw *conflictSetWorker) IsTransactionValid(t Transaction) (bool, error) {
	gn := csw.gn
	stateBytes, err := gn.Storage.Get(t.ObjectID)
	if err != nil {
		return false, fmt.Errorf("error getting state: %v", err)
	}
	if len(stateBytes) > 0 {
		var state CurrentState
		_, err := state.UnmarshalMsg(stateBytes)
		if err != nil {
			return false, fmt.Errorf("error unmarshaling state: %v", err)
		}
		stateBytes = state.Tip
	}

	stateTransition := StateTransaction{
		CurrentState: stateBytes,
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
