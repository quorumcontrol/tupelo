package node

import (
	"github.com/quorumcontrol/qc3/notary"
	"time"
	"github.com/quorumcontrol/qc3/network"
	"crypto/ecdsa"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/golang/protobuf/proto"
	"context"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/common"
	"github.com/quorumcontrol/qc3/consensus"
)

type WhisperNode struct {
	Signer *notary.Signer
	Key *ecdsa.PrivateKey
	stopChan chan bool
}

func NewWhisperNode(signer *notary.Signer, key *ecdsa.PrivateKey) *WhisperNode {
	return &WhisperNode{
		Signer: signer,
		Key: key,
		stopChan: make(chan bool),
	}
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

func historiesByChainId(chains []*consensuspb.Chain) (chainMap map[string]notary.History) {
	chainMap = make(map[string]notary.History)
	for _,chain := range chains {
		history := consensus.NewMemoryHistoryStore()
		history.StoreBlocks(chain.Blocks)
		chainMap[chain.Id]= history
	}
	return chainMap
}

func (wn *WhisperNode) handleMessage(whisp *whisper.Whisper, msg *whisper.ReceivedMessage) {
	sigRequest := &consensuspb.SignatureRequest{}
	err := proto.Unmarshal(msg.Payload, sigRequest)
	if err != nil {
		log.Error("error unmarshaling", "error", err)
		return
	}

	histories := historiesByChainId(sigRequest.Histories)

	for _,block := range sigRequest.Blocks {
		log.Debug("processing a block message")
		processed,err := wn.Signer.ProcessBlock(context.Background(), histories[block.SignableBlock.ChainId], block)
		if err != nil {
			log.Error("error processing", "error", err)
		}
		if processed != nil {
			log.Debug("processed succeeded, marshaling")
			payload,err := proto.Marshal(&consensuspb.SignatureResponse{
				Id: sigRequest.Id,
				SignerId: wn.Signer.Id(),
				Block: processed,
				Error: consensuspb.SUCCESS,
			})
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