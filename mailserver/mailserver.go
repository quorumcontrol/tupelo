package mailserver

import (
	"github.com/quorumcontrol/qc3/node"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/gogo/protobuf/proto"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/network"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/mailserver/mailserverpb"
)

var AlphaMailServerKey = crypto.Keccak256([]byte("mailserver"))

type MailServer struct {
	Mailbox *Mailbox
}

func NewMailServer(m *Mailbox) *MailServer {
	return &MailServer{
		Mailbox: m,
	}
}

func (ms *MailServer) AttachToNode(node *node.WhisperNode) {
	filter := network.NewFilter(network.CothorityTopic, AlphaMailServerKey)
	node.RegisterFilter(filter)
	node.RegisterHandler("mailserverpb.NestedEnvelope", ms.messageHandler)
}

func (ms *MailServer) messageHandler(whisp *whisper.Whisper, msg proto.Message) {
	log.Debug("message received")
	nestedEnvelope := msg.(*mailserverpb.NestedEnvelope)
	err := ms.Mailbox.Archive(nestedEnvelope)
	if err != nil {
		log.Error("error processing message")
	}
}
