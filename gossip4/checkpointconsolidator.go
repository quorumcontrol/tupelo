package gossip4

// type checkpointConslidator struct {
// 	logger   logging.EventLogger
// 	inflight inflightCheckpoints
// }

// // when a checkpoint comes in we really want to batch them up before responding to each individual
// // so we queue up the message to the end of the mailbox for responding, so we have processed
// // all the inflight checkpoints before responding to each one individually.
// type checkCheckpointAgain struct {
// 	cp        *Checkpoint
// 	requester *actor.PID
// }

// func newCheckpointConsolidator(logger logging.EventLogger) *checkpointConslidator {
// 	return &checkpointConslidator{
// 		logger:   logger,
// 		inflight: make(inflightCheckpoints),
// 	}
// }

// func newCheckpointConsolidatorProps(logger logging.EventLogger) *actor.Props {
// 	return actor.PropsFromProducer(func() actor.Actor {
// 		return newCheckpointConsolidator(logger)
// 	})
// }

// func (cc *checkpointConslidator) Receive(actorContext actor.Context) {
// 	switch msg := actorContext.Message().(type) {
// 	case *Checkpoint:
// 		conflictSet, ok := cc.inflight[msg.Height]
// 		if !ok {
// 			conflictSet = make(checkpointConflictSet)
// 		}
// 		stateKey := msg.CurrentState.String()

// 		existing, ok := conflictSet[stateKey]
// 		if !ok {
// 			cc.logger.Debugf("consolidator: no existing checkpoint, creating")
// 			conflictSet[stateKey] = msg
// 			cc.inflight[msg.Height] = conflictSet

// 			// see note on the checkCheckpointAgain struct
// 			if sender := actorContext.Sender(); sender != nil {
// 				actorContext.Send(actorContext.Self(), &checkCheckpointAgain{
// 					cp:        msg,
// 					requester: sender,
// 				})
// 			}

// 			return
// 		}

// 		// otherwise we have an existing, lets see if we should combine them
// 		_, theirs := signerDiff(existing.Signature, msg.Signature)
// 		if theirs > 0 {
// 			cc.logger.Debugf("consolidator: new signatures on checkpoint")

// 			// then we do have a new signature here!
// 			newSig, err := sigfuncs.AggregateBLSSignatures([]*signatures.Signature{&existing.Signature, &msg.Signature})
// 			if err != nil {
// 				cc.logger.Warningf("error aggregating: %v", err)
// 				return
// 			}
// 			existing.Signature = *newSig
// 			conflictSet[existing.CurrentState.String()] = existing
// 			cc.inflight[msg.Height] = conflictSet

// 			// see note on the checkCheckpointAgain struct
// 			// in this case we send the existing because it has the new signature on it
// 			// when the queue of messages is processed the checkCheckpointAgain will look
// 			// to see if *this* existing is the best and if so, will allow the broadcast
// 			if sender := actorContext.Sender(); sender != nil {
// 				actorContext.Send(actorContext.Self(), &checkCheckpointAgain{
// 					cp:        existing,
// 					requester: sender,
// 				})
// 			}

// 		}
// 	case *checkCheckpointAgain:
// 		conflictSet, ok := cc.inflight[msg.cp.Height]
// 		if !ok {
// 			cc.logger.Debugf("consolidator: checkpoint missing, probably committed")
// 			conflictSet = make(checkpointConflictSet)
// 			// we've already committed, so no need to keep broadcasting this Checkpoint
// 			actorContext.Send(msg.requester, false)
// 			return
// 		}
// 		stateKey := msg.cp.CurrentState.String()

// 		existing, ok := conflictSet[stateKey]
// 		if ok {
// 			ours, _ := signerDiff(existing.Signature, msg.cp.Signature)
// 			if ours == 0 {
// 				cc.logger.Debugf("consolidator: we haven't added any more signatures, allow to propogate")
// 				// a 0 here means that even though we batched up all these requests
// 				// the one they sent was still the most helpful, so let the broadcast continue
// 				actorContext.Send(msg.requester, true)
// 				return
// 			}
// 		}
// 		// if we got here then either we've received better checkpoints *or* we've already committed
// 		// and there's no need to keep broadcasting that message that led us to here
// 		actorContext.Send(msg.requester, false)
// 	}
// }
