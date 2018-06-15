package node

import (
	"context"
	"crypto/ecdsa"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/gogo/protobuf/proto"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/notary"
)

type MessageHandlerFunc func(whisp *whisper.Whisper, msg proto.Message, metadata *MessageMetadata)

type MessageMetadata struct {
	Src        *ecdsa.PublicKey
	Dst        *ecdsa.PublicKey
	MessageId  []byte
	ProtocolId string
}

type WhisperNode struct {
	Signer      *notary.Signer
	Key         *ecdsa.PrivateKey
	stopChan    chan bool
	handlers    map[string]MessageHandlerFunc
	messageChan chan *whisper.ReceivedMessage
	filters     []*whisper.Filter
}

func NewWhisperNode(signer *notary.Signer, key *ecdsa.PrivateKey) *WhisperNode {
	wn := &WhisperNode{
		Signer:      signer,
		Key:         key,
		stopChan:    make(chan bool, 1),
		handlers:    make(map[string]MessageHandlerFunc),
		messageChan: make(chan *whisper.ReceivedMessage),
		filters:     []*whisper.Filter{network.NewFilter(network.CothorityTopic, crypto.Keccak256([]byte(signer.Group.Id)))},
	}
	wn.RegisterHandler(proto.MessageName(&consensuspb.SignatureRequest{}), wn.handleSignatureRequest)
	wn.RegisterHandler(proto.MessageName(&consensuspb.TipRequest{}), wn.handleTipRequest)
	return wn
}

func (wn *WhisperNode) RegisterHandler(typeName string, function MessageHandlerFunc) {
	log.Debug("resgistering handler", "typeName", typeName)
	wn.handlers[typeName] = function
}

func (wn *WhisperNode) RegisterFilter(filter *whisper.Filter) {
	wn.filters = append(wn.filters, filter)
}

func (wn *WhisperNode) Start() {
	ticker := time.NewTicker(100 * time.Millisecond)
	log.Debug("starting node whisper")
	whisp := network.Start(wn.Key)
	for _, filter := range wn.filters {
		whisp.Subscribe(filter)
	}

	//TODO: this is shitty paralellization - should have workers , etc
	// see https://nesv.github.io/golang/2014/02/25/worker-queues-in-go.html or
	// other better job distribution
	go func() {
		for {
			select {
			case msg := <-wn.messageChan:
				log.Debug("received message", "message", msg, "stats", whisp.Stats())
				wn.handleMessage(whisp, msg)
			case <-wn.stopChan:
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ticker.C:
				for _, filter := range wn.filters {
					msgs := filter.Retrieve()
					for _, msg := range msgs {
						wn.messageChan <- msg
					}
				}
			case <-wn.stopChan:
				whisp.Stop()
				return
			}
		}
	}()
}

func (wn *WhisperNode) Stop() {
	wn.stopChan <- true
	wn.stopChan <- true
}

func historiesByChainId(chains []*consensuspb.Chain) (chainMap map[string]consensus.History) {
	chainMap = make(map[string]consensus.History)
	for _, chain := range chains {
		history := consensus.NewMemoryHistoryStore()
		history.StoreBlocks(chain.Blocks)
		chainMap[chain.Id] = history
	}
	return chainMap
}

func (wn *WhisperNode) handleTipRequest(whisp *whisper.Whisper, tipRequestMsg proto.Message, metadata *MessageMetadata) {
	tipRequest := tipRequestMsg.(*consensuspb.TipRequest)
	tip, err := wn.Signer.GetTip(context.Background(), tipRequest.ChainId)
	if err != nil {
		log.Error("error getting tip", "error", err)
		return
	}

	payload, err := proto.Marshal(&consensuspb.ProtocolResponse{
		Id:       metadata.ProtocolId,
		Response: consensus.ObjToAny(tip),
	})
	if err != nil {
		log.Error("error marshaling", "error", err)
	}

	err = network.Send(whisp, &whisper.MessageParams{
		TTL:      60 * 60, // 1 hour, TODO: what are the right TTL settings?
		Dst:      metadata.Src,
		Src:      wn.Key,
		Topic:    whisper.BytesToTopic(network.CothorityTopic),
		PoW:      .02, // TODO: what are the right settings for PoW?
		WorkTime: 10,
		Payload:  payload,
	})

	if err != nil {
		log.Error("error sending tip request message", "error", err)
	}

}

func (wn *WhisperNode) handleSignatureRequest(whisp *whisper.Whisper, sigRequestMsg proto.Message, metadata *MessageMetadata) {
	sigRequest := sigRequestMsg.(*consensuspb.SignatureRequest)
	histories := historiesByChainId(sigRequest.Histories)

	for _, block := range sigRequest.Blocks {
		log.Debug("processing a block message")
		processed, err := wn.Signer.ProcessBlock(context.Background(), histories[block.SignableBlock.ChainId], block)
		if err != nil {
			log.Error("error processing", "error", err)
		}
		if processed != nil {
			log.Debug("processed succeeded, marshaling")
			resp := &consensuspb.ProtocolResponse{
				Id: metadata.ProtocolId,
				Response: consensus.ObjToAny(&consensuspb.SignatureResponse{
					SignerId: wn.Signer.Id(),
					Block:    processed,
					Error:    consensuspb.SUCCESS,
				}),
			}
			if err != nil {
				log.Error("error converting to any", "error", err)
			}

			payload, err := proto.Marshal(resp)
			if err != nil {
				log.Error("error marshaling", "error", err)
			}
			log.Debug("sending signed response", "topic", whisper.BytesToTopic(network.CothorityTopic), "key", crypto.ToECDSAPub(sigRequest.ResponseKey), "bytes", sigRequest.ResponseKey)
			err = network.Send(whisp, &whisper.MessageParams{
				TTL:      60 * 60, // 1 hour, TODO: what are the right TTL settings?
				Dst:      crypto.ToECDSAPub(sigRequest.ResponseKey),
				Src:      wn.Key,
				Topic:    whisper.BytesToTopic(network.CothorityTopic),
				PoW:      .02, // TODO: what are the right settings for PoW?
				WorkTime: 10,
				Payload:  payload,
			})
			if err != nil {
				log.Error("error sending", "error", err)
			} else {
				log.Debug("message sent")
			}
		} else {
			log.Error("did not process block", "block", block)
		}
	}
}

func (wn *WhisperNode) handleMessage(whisp *whisper.Whisper, msg *whisper.ReceivedMessage) {
	protocolRequest := &consensuspb.ProtocolRequest{}
	err := proto.Unmarshal(msg.Payload, protocolRequest)
	if err != nil {
		log.Error("error unmarshaling", "error", err)
		return
	}

	handlerFunc, ok := wn.handlers[protocolRequest.Request.TypeUrl]
	if ok {
		metadata := &MessageMetadata{
			Src:        msg.Src,
			Dst:        msg.Dst,
			MessageId:  msg.EnvelopeHash.Bytes(),
			ProtocolId: protocolRequest.Id,
		}
		obj, err := consensus.AnyToObj(protocolRequest.Request)
		if err != nil {
			log.Error("error converting any to obj", "error", err)
		}
		handlerFunc(whisp, obj, metadata)
	} else {
		log.Info("unknown message type received", "type", protocolRequest.Request.TypeUrl)
	}

}
