package node

import (
	"github.com/quorumcontrol/qc3/notary"
	"time"
	"github.com/quorumcontrol/qc3/network"
	"crypto/ecdsa"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"context"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/common"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/gogo/protobuf/types"
	"reflect"
	"github.com/gogo/protobuf/proto"
)

type MessageHandlerFunc func(whisp *whisper.Whisper, msg proto.Message)

type WhisperNode struct {
	Signer *notary.Signer
	Key *ecdsa.PrivateKey
	stopChan chan bool
	handlers map[string]MessageHandlerFunc
}

func NewWhisperNode(signer *notary.Signer, key *ecdsa.PrivateKey) *WhisperNode {
	wn := &WhisperNode{
		Signer: signer,
		Key: key,
		stopChan: make(chan bool),
		handlers: make(map[string]MessageHandlerFunc),
	}
	wn.RegisterHandler(proto.MessageName(&consensuspb.SignatureRequest{}), wn.handleSignatureRequest)
	return wn
}

func (wn *WhisperNode) RegisterHandler(typeName string, function MessageHandlerFunc) {
	log.Debug("resgistering handler", "typeName", typeName)
	wn.handlers[typeName] = function
}

func (wn *WhisperNode) Start() {
	ticker := time.NewTicker(100 * time.Millisecond)
	log.Debug("starting node whisper")
	whisp := network.Start(wn.Key)
	filter := network.NewFilter(network.CothorityTopic, common.StringToHash(wn.Signer.Group.Id).Bytes())
	whisp.Subscribe(filter)

	go func() {
		for {
			select {
			case <-ticker.C:
				msgs := filter.Retrieve()
				for _,msg := range msgs {
					//TODO: parallel processing
					log.Debug("received message", "message", msg, "stats", whisp.Stats())
					wn.handleMessage(whisp, msg)
				}
			case <-wn.stopChan:
				whisp.Stop()
				return
			}
		}
	}()
}

func (wn *WhisperNode) Stop() {
	wn.stopChan<-true
}

func historiesByChainId(chains []*consensuspb.Chain) (chainMap map[string]consensus.History) {
	chainMap = make(map[string]consensus.History)
	for _,chain := range chains {
		history := consensus.NewMemoryHistoryStore()
		history.StoreBlocks(chain.Blocks)
		chainMap[chain.Id]= history
	}
	return chainMap
}

func (wn *WhisperNode) handleSignatureRequest(whisp *whisper.Whisper, sigRequestMsg proto.Message) {
	sigRequest := sigRequestMsg.(*consensuspb.SignatureRequest)
	histories := historiesByChainId(sigRequest.Histories)

	for _,block := range sigRequest.Blocks {
		log.Debug("processing a block message")
		processed,err := wn.Signer.ProcessBlock(context.Background(), histories[block.SignableBlock.ChainId], block)
		if err != nil {
			log.Error("error processing", "error", err)
		}
		if processed != nil {
			log.Debug("processed succeeded, marshaling")
			any,err := objToAny(&consensuspb.SignatureResponse{
				Id: sigRequest.Id,
				SignerId: wn.Signer.Id(),
				Block: processed,
				Error: consensuspb.SUCCESS,
			})
			if err != nil {
				log.Error("error converting to any", "error", err)
			}

			payload,err := proto.Marshal(any)
			if err != nil {
				log.Error("error marshaling", "error", err)
			}
			log.Debug("sending signed response", "topic", whisper.BytesToTopic(network.CothorityTopic), "key", crypto.ToECDSAPub(sigRequest.ResponseKey), "bytes", sigRequest.ResponseKey)
			err = network.Send(whisp, &whisper.MessageParams{
				TTL: 60*60, // 1 hour, TODO: what are the right TTL settings?
				Dst: crypto.ToECDSAPub(sigRequest.ResponseKey),
				Src: wn.Key,
				Topic: whisper.BytesToTopic(network.CothorityTopic),
				PoW: .02,  // TODO: what are the right settings for PoW?
				WorkTime: 10,
				Payload: payload,
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
	any := &types.Any{}
	err := proto.Unmarshal(msg.Payload, any)
	if err != nil {
		log.Error("error unmarshaling", "error", err)
		return
	}

	handlerFunc,ok := wn.handlers[any.TypeUrl]
	if ok {
		obj,err := anyToObj(any)
		if err != nil {
			log.Error("error converting any to obj", "error", err)
		}
		handlerFunc(whisp, obj)
	} else {
		log.Info("unknown message type received", "type", any.TypeUrl)
	}

}

func anyToObj(any *types.Any) (proto.Message, error) {
	typeName := any.TypeUrl
	instanceType := proto.MessageType(typeName)
	log.Debug("unmarshaling from Any type to type: %v from typeName %s", "type", instanceType, "name", typeName)

	// instanceType will be a pointer type, so call Elem() to get the original Type and then interface
	// so that we can change it to the kind of object we want
	instance := reflect.New(instanceType.Elem()).Interface()
	err := proto.Unmarshal(any.GetValue(), instance.(proto.Message))
	if err != nil {
		return nil, err
	}
	return instance.(proto.Message), nil
}


func objToAny(obj proto.Message) (*types.Any, error) {
	objectType := proto.MessageName(obj)
	bytes, err := proto.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return &types.Any{
		TypeUrl: objectType,
		Value:   bytes,
	}, nil
}