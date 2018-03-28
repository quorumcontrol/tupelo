package mailserver

import (
	"github.com/quorumcontrol/qc3/node"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/gogo/protobuf/proto"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/network"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/mailserver/mailserverpb"
	"github.com/ethereum/go-ethereum/rlp"
	"fmt"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/consensus"
)

var AlphaMailServerKey = crypto.Keccak256([]byte("mailserver"))

// TODO: there is no security on replay or ack
// the replay isn't such a risk since the envelope is secured, but could be a DDoS attack
// the ack could delete people's messages (but the attacker would need to know the message hash)

type MailServer struct {
	Mailbox *Mailbox
	node *node.WhisperNode
}

func NewMailServer(m *Mailbox) *MailServer {
	return &MailServer{
		Mailbox: m,
	}
}

func (ms *MailServer) AttachToNode(node *node.WhisperNode) {
	ms.node = node
	filter := network.NewFilter(network.CothorityTopic, AlphaMailServerKey)
	node.RegisterFilter(filter)
	node.RegisterHandler(proto.MessageName(&mailserverpb.NestedEnvelope{}), ms.archiveHandler)
	node.RegisterHandler(proto.MessageName(&mailserverpb.AckEnvelope{}), ms.ackHandler)
	node.RegisterHandler(proto.MessageName(&mailserverpb.ReplayRequest{}), ms.replayHandler)
}

func (ms *MailServer) archiveHandler(whisp *whisper.Whisper, msg proto.Message, metadata *node.MessageMetadata) {
	log.Debug("message received")
	nestedEnvelope := msg.(*mailserverpb.NestedEnvelope)
	err := ms.Mailbox.Archive(nestedEnvelope)
	if err != nil {
		log.Error("error processing message")
	}
}

func (ms *MailServer) replayHandler(whisp *whisper.Whisper, msg proto.Message, metadata *node.MessageMetadata) {
	replayMessage := msg.(*mailserverpb.ReplayRequest)

	log.Debug("replay received", "destination", replayMessage.Destination)

	ms.Mailbox.ForEach(replayMessage.Destination, func (env *whisper.Envelope) error {
		envBytes, err := rlp.EncodeToBytes(env)
		if err != nil {
			return fmt.Errorf("error encoding: %v", err)
		}

		response := &consensuspb.ProtocolResponse{
			Id: metadata.ProtocolId,
			Response: consensus.ObjToAny(&mailserverpb.ReplayResponse{
				Envelope: envBytes,
			}),
		}

		payload,err := proto.Marshal(response)
		if err != nil {
			return fmt.Errorf("error encoding: %v", err)
		}

		log.Debug("sending back message with protocol id", "protocolId", response.Id)

		network.Send(whisp, &whisper.MessageParams{
			TTL: 60, // 1 minute TODO: what are the right TTL settings?
			Dst: metadata.Src,
			Src: ms.node.Key,
			Topic: whisper.BytesToTopic(network.CothorityTopic),
			PoW: .02,  // TODO: what are the right settings for PoW?
			WorkTime: 10,
			Payload: payload,
		})
		return nil
	})
}

func (ms *MailServer) ackHandler(whisp *whisper.Whisper, msg proto.Message, metadata *node.MessageMetadata) {
	ackMessage := msg.(*mailserverpb.AckEnvelope)
	log.Debug("acking", "destination", ackMessage.Destination, "hash", ackMessage.EnvelopeHash)
	ms.Mailbox.Ack(ackMessage.Destination, ackMessage.EnvelopeHash)
}
