package signer

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/quorumcontrol/chaintree/safewrap"

	"github.com/quorumcontrol/chaintree/typecaster"

	"github.com/quorumcontrol/qc3/bls"

	"fmt"

	"sync"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossip"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/storage"
)

type pendingResponse struct {
	ch network.ResponseChan
	id string
}

type responseHolder map[string]*pendingResponse

type byAddress []*consensus.RemoteNode

func (a byAddress) Len() int      { return len(a) }
func (a byAddress) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byAddress) Less(i, j int) bool {
	return a[i].Id < a[j].Id
}

// type roundTransactionHolder map[string]

// GossipedSigner uses the gosig based gossip protocol
// in order to reach consensus.
type GossipedSigner struct {
	gossiper    *gossip.Gossiper
	started     bool
	responses   responseHolder
	respLock    *sync.RWMutex
	roundInfos  map[int64]*consensus.RoundInfo
	roundBlocks map[int64]*cid.Cid
}

// GroupToTopic takes a NotaryGroup and returns the Whisper Topic to reach it.
func GroupToTopic(group *consensus.NotaryGroup) []byte {
	return []byte(group.ID)
}

func NewGossipedSigner(node *network.Node, group *consensus.NotaryGroup, store storage.Storage, signKey *bls.SignKey) *GossipedSigner {
	if group == nil {
		panic("invalid group")
	}

	gossipSigner := &GossipedSigner{
		responses:   make(responseHolder),
		respLock:    &sync.RWMutex{},
		roundInfos:  make(map[int64]*consensus.RoundInfo),
		roundBlocks: make(map[int64]*cid.Cid),
	}

	handler := network.NewMessageHandler(node, GroupToTopic(group))

	gossiper := &gossip.Gossiper{
		MessageHandler:  handler,
		SignKey:         signKey,
		Group:           group,
		Storage:         store,
		StateHandler:    gossipSigner.stateHandler,
		AcceptedHandler: gossipSigner.acceptedHandler,
		Fanout:          5,
	}
	gossiper.Initialize()

	gossipSigner.gossiper = gossiper
	handler.AssignHandler(consensus.MessageType_AddBlock, gossipSigner.AddBlockHandler)
	handler.AssignHandler(consensus.MessageType_TipRequest, gossipSigner.TipHandler)
	gossiper.AddRoundHandler(gossipSigner.roundHandler)

	return gossipSigner
}

func (gs *GossipedSigner) Start() {
	if gs.started {
		return
	}
	gs.started = true
	gs.gossiper.Start()
}

func (gs *GossipedSigner) Stop() {
	if !gs.started {
		return
	}
	gs.started = false
	gs.gossiper.Stop()
}

func (gs *GossipedSigner) stateHandler(ctx context.Context, stateTrans gossip.StateTransaction) (nextState []byte, accepted bool, err error) {
	addBlockrequest := &consensus.AddBlockRequest{}
	err = cbornode.DecodeInto(stateTrans.Transaction, addBlockrequest)
	if err != nil {
		return nil, false, fmt.Errorf("error getting payload: %v", err)
	}

	var storedTip *cid.Cid

	if len(stateTrans.State) > 1 {
		storedTip, err = cid.Cast(stateTrans.State)
		if err != nil {
			log.Error("error casting state into CID", "err", err)
			return nil, false, &consensus.ErrorCode{Memo: fmt.Sprintf("error casting: %v", err), Code: consensus.ErrUnknown}
		}
	}

	var resp *consensus.AddBlockResponse

	if addBlockrequest.ChainId == gs.gossiper.Group.ID {
		log.Debug("chaintree block incoming")
		dataPayload := &consensus.SetDataPayload{}
		typecaster.ToType(addBlockrequest.NewBlock.Transactions[0].Payload, dataPayload)
		roundStrings := strings.Split(dataPayload.Path, "rounds/")
		round, err := strconv.Atoi(roundStrings[len(roundStrings)-1])
		if err != nil {
			return nil, false, &consensus.ErrorCode{Memo: fmt.Sprintf("error casting: %v", err), Code: consensus.ErrUnknown}
		}
		sw := &safewrap.SafeWrap{}
		node := sw.Decode(stateTrans.Transaction)
		existing, ok := gs.roundBlocks[int64(round)]
		if !ok {
			calculated := gs.calculateRequestForRound(int64(round))
			calculatedNode := sw.WrapObject(calculated)
			existing = calculatedNode.Cid()
			gs.roundBlocks[int64(round)] = existing
		}
		if node.Cid().Equals(existing) {
			expected, err := gs.gossiper.Group.ExpectedTipWithBlock(addBlockrequest.NewBlock)
			if err != nil {
				return nil, false, fmt.Errorf("error processing block: %v", err)
			}
			return expected.Bytes(), true, nil
		}
		log.Error("error,existing did not match %s %s", existing.String(), node.Cid().String())
		return nil, false, nil

		// do special handling of notary group
		// if this new block matches the block I have for the round of the block
		// then we can approve the block
	} else {
		resp, err = processAddBlock(storedTip, addBlockrequest)
		if err != nil {
			log.Error("error processing block", "err", err)
			return nil, false, nil
		}
	}

	return resp.Tip.Bytes(), true, nil
}

func (gs *GossipedSigner) calculateRequestForRound(round int64) *consensus.AddBlockRequest {
	roundInfo := gs.roundInfos[round]
	if roundInfo == nil {
		roundInfo = &consensus.RoundInfo{
			Round: round,
		}
	}
	previousInfo, err := gs.gossiper.Group.RoundInfoFor(round - 1)
	if err != nil {
		panic(fmt.Sprintf("error finding previous round: %d", round-1))
	}
	roundInfo.Signers = append(roundInfo.Signers, previousInfo.Signers...)
	sort.Sort(byAddress(roundInfo.Signers))

	block, err := gs.gossiper.Group.CreateBlockFor(round, roundInfo.Signers)
	if err != nil {
		panic(fmt.Sprintf("error creating block:%v", err))
	}

	addBlockRequest := &consensus.AddBlockRequest{
		ChainId:  gs.gossiper.Group.ID,
		NewBlock: block,
		Tip:      gs.gossiper.Group.Tip(),
	}
	return addBlockRequest
}

func (gs *GossipedSigner) roundHandler(ctx context.Context, round int64) {
	// create block for round + 6 and gossip that block around
	// gossip block
	log.Debug("round handler", "round", round)
	_, ok := gs.roundBlocks[round+6]
	if !ok {
		// no need to handle this round, it's already in the chaintree
		roundInfo, _ := gs.gossiper.Group.RoundInfoFor(round + 6)
		if roundInfo != nil {
			return
		}

		addBlockRequest := gs.calculateRequestForRound(round + 6)

		sw := &safewrap.SafeWrap{}
		cborNode := sw.WrapObject(addBlockRequest)
		gs.roundBlocks[round+6] = cborNode.Cid()

		gossipMessage := &gossip.GossipMessage{
			ObjectID:    []byte(addBlockRequest.ChainId),
			Transaction: cborNode.RawData(),
			PreviousTip: []byte(addBlockRequest.NewBlock.PreviousTip),
			Phase:       0,
			Round:       gs.gossiper.Group.RoundAt(time.Now()),
		}

		err := gs.gossiper.HandleGossip(ctx, gossipMessage)
		if err != nil {
			panic(fmt.Sprintf("error handling request: %v", err))
		}
	}
}

func (gs *GossipedSigner) acceptedHandler(ctx context.Context, acceptedTransaction gossip.StateTransaction) (err error) {
	log.Debug("accepted handler")
	addBlockrequest := &consensus.AddBlockRequest{}
	err = cbornode.DecodeInto(acceptedTransaction.Transaction, addBlockrequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}
	if addBlockrequest.ChainId == gs.gossiper.Group.ID {
		log.Debug("add block to chaintree")
		gs.gossiper.Group.AddBlock(addBlockrequest.NewBlock)
		return nil
	}
	err = gs.respondToTransaction(acceptedTransaction.ObjectID, acceptedTransaction.Transaction)
	if err != nil {
		return fmt.Errorf("error responding to trans: %v", err)
	}

	if trans := stakeTransactionFromBlock(addBlockrequest.NewBlock); trans != nil {
		log.Debug("new stake")

		stakePayload := &consensus.StakePayload{}
		err = typecaster.ToType(trans.Payload, stakePayload)
		if err != nil {
			return fmt.Errorf("error casting: %v", err)
		}
		remoteNode := consensus.NewRemoteNode(stakePayload.VerKey, stakePayload.DstKey)
		roundInfo := gs.roundInfos[acceptedTransaction.Round+6]
		if roundInfo == nil {
			roundInfo = &consensus.RoundInfo{
				Round: acceptedTransaction.Round + 6,
			}
		}
		roundInfo.Signers = append(roundInfo.Signers, remoteNode)
		gs.roundInfos[acceptedTransaction.Round+6] = roundInfo
	}

	return nil
}

func (gs *GossipedSigner) AddBlockHandler(ctx context.Context, addBlockNetworkReq network.Request, respChan network.ResponseChan) error {
	addBlockrequest := &consensus.AddBlockRequest{}
	err := cbornode.DecodeInto(addBlockNetworkReq.Payload, addBlockrequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("add block handler", "tip", addBlockrequest.Tip, "request", addBlockrequest)

	gossipMessage := &gossip.GossipMessage{
		ObjectID:    []byte(addBlockrequest.ChainId),
		Transaction: addBlockNetworkReq.Payload,
		PreviousTip: []byte(addBlockrequest.NewBlock.PreviousTip),
		Phase:       0,
		Round:       gs.gossiper.Group.RoundAt(time.Now()),
	}

	pending := &pendingResponse{
		id: addBlockNetworkReq.Id,
		ch: respChan,
	}

	gs.respLock.Lock()
	gs.responses[string(crypto.Keccak256(addBlockNetworkReq.Payload))] = pending
	gs.respLock.Unlock()

	err = gs.gossiper.HandleGossip(ctx, gossipMessage)
	if err != nil {
		return fmt.Errorf("error handling request")
	}

	return nil
}

func (gs *GossipedSigner) TipHandler(_ context.Context, req network.Request, respChan network.ResponseChan) error {
	tipRequest := &consensus.TipRequest{}
	err := cbornode.DecodeInto(req.Payload, tipRequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	tipResponse, err := gs.tipForObject([]byte(tipRequest.ChainId))
	if err != nil {
		return fmt.Errorf("error getting tip: %v", err)
	}

	resp, err := network.BuildResponse(req.Id, 200, tipResponse)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}

	respChan <- resp

	return nil
}

func (gs *GossipedSigner) respondToTransaction(objectId, transaction []byte) error {
	log.Debug("respondToTransaction")
	transKey := string(crypto.Keccak256(transaction))

	gs.respLock.RLock()
	pending, ok := gs.responses[transKey]
	if ok {
		log.Debug("pending found, responding")
		gs.respLock.RUnlock()
		tipResponse, err := gs.tipForObject(objectId)
		if err != nil {
			return fmt.Errorf("error getting tip: %v", err)
		}

		resp, err := network.BuildResponse(pending.id, 200, tipResponse)
		if err != nil {
			return fmt.Errorf("error building response: %v", err)
		}

		pending.ch <- resp

		gs.respLock.Lock()
		defer gs.respLock.Unlock()
		delete(gs.responses, transKey)
	} else {
		gs.respLock.RUnlock()
	}
	return nil
}

func (gs *GossipedSigner) tipForObject(objectId []byte) (*consensus.TipResponse, error) {
	currState, err := gs.gossiper.GetCurrentState(objectId)
	if err != nil {
		return nil, fmt.Errorf("error getting state: %v", err)
	}

	var tip *cid.Cid

	if len(currState.Tip) > 0 {
		tip, err = cid.Cast(currState.Tip)
		if err != nil {
			return nil, fmt.Errorf("error casting tip: %v", err)
		}
	}

	return &consensus.TipResponse{
		ChainId:   string(objectId),
		Tip:       tip,
		Signature: currState.Signature,
	}, nil
}

func stakeTransactionFromBlock(block *chaintree.BlockWithHeaders) *chaintree.Transaction {
	for _, trans := range block.Block.Transactions {
		if trans.Type == consensus.TransactionTypeStake {
			return trans
		}
	}
	return nil
}
