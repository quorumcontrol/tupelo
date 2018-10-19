package signer

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossip"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/storage"
)

const RoundsUntilClosed = 2
const RoundsUntilNewSignerIsLive = RoundsUntilClosed + 4

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

type PendingGroupChange struct {
	round    int64
	addition *consensus.RemoteNode
}

// GossipedSigner uses the gosig based gossip protocol
// in order to reach consensus.
type GossipedSigner struct {
	gossiper            *gossip.Gossiper
	started             bool
	responses           responseHolder
	respLock            *sync.RWMutex
	roundBlocks         map[int64]*cid.Cid
	pendingGroupChanges map[int64][]*PendingGroupChange
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
		responses:           make(responseHolder),
		respLock:            &sync.RWMutex{},
		roundBlocks:         make(map[int64]*cid.Cid),
		pendingGroupChanges: make(map[int64][]*PendingGroupChange),
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
	handler.AssignHandler(consensus.MessageType_GetDiffNodes, gossipSigner.GetDiffNodes)
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

		currentRound := gs.gossiper.Group.RoundAt(time.Now())
		if int64(round) > (currentRound + 5) {
			return nil, false, &consensus.ErrorCode{Memo: "error, can not process chaintree change further than 5 rounds in the future", Code: consensus.ErrUnknown}
		}

		sw := &safewrap.SafeWrap{}

		existing, _ := gs.roundBlocks[int64(round)]
		if existing == nil {
			gs.calculateBlockForRound(int64(round))
			existing, _ = gs.roundBlocks[int64(round)]
		}

		if existing == nil {
			return nil, false, &consensus.ErrorCode{Memo: fmt.Sprintf("could not calculate round info for %v", round), Code: consensus.ErrUnknown}
		}

		node := sw.Decode(stateTrans.Transaction)
		if sw.Err != nil {
			return nil, false, &consensus.ErrorCode{Memo: fmt.Sprintf("error casting: %v", sw.Err), Code: consensus.ErrUnknown}
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
	}

	if len(gs.processableGroupChanges(stateTrans.Round)) > 0 {
		log.Error("cannot process block, notarygroup has pending changes", "err", err)
		return nil, false, nil
	}

	resp, err = processAddBlock(storedTip, addBlockrequest)
	if err != nil {
		log.Error("error processing block", "err", err)
		return nil, false, nil
	}

	return resp.Tip.Bytes(), true, nil
}

func (gs *GossipedSigner) processableGroupChanges(round int64) (pendingChanges []*PendingGroupChange) {
	for changeRound, changes := range gs.pendingGroupChanges {
		if changeRound <= (round - RoundsUntilNewSignerIsLive) {
			pendingChanges = append(pendingChanges, changes...)
		}
	}
	return pendingChanges
}

func (gs *GossipedSigner) calculateBlockForRound(round int64) (*consensus.AddBlockRequest, bool) {
	roundInfo, err := gs.gossiper.Group.MostRecentRoundInfo(round - 1)
	if err != nil {
		panic(fmt.Sprintf("error finding previous round: %d", round-1))
	}

	pendingChanges := gs.processableGroupChanges(round)

	hadChangesFromLast := false

	if len(pendingChanges) > 0 {
		for _, change := range pendingChanges {
			if change.addition != nil && !roundInfo.HasMember(&change.addition.VerKey) {
				hadChangesFromLast = true
				roundInfo.Signers = append(roundInfo.Signers, change.addition)
			}
		}
		sort.Sort(byAddress(roundInfo.Signers))
	}

	block, err := gs.gossiper.Group.CreateBlockFor(round, roundInfo.Signers)
	if err != nil {
		panic(fmt.Sprintf("error creating round block: %v", err))
	}

	addBlockRequest := &consensus.AddBlockRequest{
		ChainId:  gs.gossiper.Group.ID,
		NewBlock: block,
		Tip:      gs.gossiper.Group.Tip(),
	}

	sw := &safewrap.SafeWrap{}
	cborNode := sw.WrapObject(addBlockRequest)
	gs.roundBlocks[round] = cborNode.Cid()

	return addBlockRequest, hadChangesFromLast
}

func (gs *GossipedSigner) roundHandler(ctx context.Context, round int64) {
	// Cleanup old round blocks that are no longer used
	_, ok := gs.roundBlocks[round-RoundsUntilClosed]
	if ok {
		delete(gs.roundBlocks, (round - RoundsUntilClosed))
	}

	targetRound := round + (RoundsUntilNewSignerIsLive - RoundsUntilClosed)

	roundInfo, _ := gs.gossiper.Group.RoundInfoFor(targetRound)
	if roundInfo != nil {
		return
	}

	existing, _ := gs.roundBlocks[targetRound]
	if existing != nil {
		return
	}

	addBlockRequest, hadChanges := gs.calculateBlockForRound(targetRound)

	if hadChanges {
		sw := &safewrap.SafeWrap{}
		cborNode := sw.WrapObject(addBlockRequest)

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
	addBlockrequest := &consensus.AddBlockRequest{}
	err = cbornode.DecodeInto(acceptedTransaction.Transaction, addBlockrequest)
	log.Debug("accepted handler", "objid", string(acceptedTransaction.ObjectID), "action", addBlockrequest.NewBlock.Block.Transactions[0].Type)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}
	if addBlockrequest.ChainId == gs.gossiper.Group.ID {
		log.Debug("add block to chaintree")
		gs.gossiper.Group.AddBlock(addBlockrequest.NewBlock)

		for _, transaction := range addBlockrequest.NewBlock.Transactions {
			payload := transaction.Payload.(map[string]interface{})
			roundStrings := strings.Split(payload["path"].(string), "rounds/")
			round, _ := strconv.Atoi(roundStrings[len(roundStrings)-1])

			if _, ok := gs.pendingGroupChanges[int64(round)]; ok {
				delete(gs.pendingGroupChanges, int64(round))
			}
		}

		return nil
	}
	err = gs.respondToTransaction(acceptedTransaction.ObjectID, acceptedTransaction.Transaction)
	if err != nil {
		return fmt.Errorf("error responding to trans: %v", err)
	}

	if trans := stakeTransactionFromBlock(addBlockrequest.NewBlock); trans != nil {
		log.Debug("new stake", "g", gs.gossiper.ID, "staker", addBlockrequest.ChainId, "targetround", acceptedTransaction.Round+6)

		stakePayload := &consensus.StakePayload{}
		err = typecaster.ToType(trans.Payload, stakePayload)
		if err != nil {
			return fmt.Errorf("error casting: %v", err)
		}
		remoteNode := consensus.NewRemoteNode(stakePayload.VerKey, stakePayload.DstKey)

		if _, ok := gs.pendingGroupChanges[acceptedTransaction.Round]; !ok {
			gs.pendingGroupChanges[acceptedTransaction.Round] = make([]*PendingGroupChange, 0)
		}

		gs.pendingGroupChanges[acceptedTransaction.Round] = append(gs.pendingGroupChanges[acceptedTransaction.Round], &PendingGroupChange{
			round:    acceptedTransaction.Round,
			addition: remoteNode,
		})
	}

	return nil
}

func (gs *GossipedSigner) GetDiffNodes(ctx context.Context, networkReq network.Request, respChan network.ResponseChan) error {
	diffRequest := &consensus.GetDiffNodesRequest{}
	err := cbornode.DecodeInto(networkReq.Payload, diffRequest)
	if err != nil {
		return fmt.Errorf("error getting request: %v", err)
	}

	newNodes, err := gs.gossiper.Group.NodesAt(diffRequest.NewTip)
	if err != nil {
		return fmt.Errorf("error getting current nodes: %v", err)
	}

	previousNodes := make([]*cbornode.Node, 0)

	previousTipNode, err := gs.gossiper.Group.GetNode(diffRequest.PreviousTip)
	if previousTipNode != nil {
		previousNodes, err = gs.gossiper.Group.NodesAt(diffRequest.PreviousTip)
		if err != nil {
			return fmt.Errorf("error getting previous nodes: %v", err)
		}
	}

	previousNodedsByCid := make(map[*cid.Cid]*cbornode.Node)

	for _, node := range previousNodes {
		previousNodedsByCid[node.Cid()] = node
	}

	nodesResp := &consensus.GetDiffNodesResponse{
		Nodes: make([][]byte, 0),
	}

	for _, node := range newNodes {
		_, ok := previousNodedsByCid[node.Cid()]
		if !ok {
			nodesResp.Nodes = append(nodesResp.Nodes, node.RawData())
		}
	}

	netResp, err := network.BuildResponse(networkReq.Id, 200, nodesResp)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}
	respChan <- netResp
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
