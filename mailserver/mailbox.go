package mailserver

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/common"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/quorumcontrol/qc3/mailserver/mailserverpb"
	"github.com/ethereum/go-ethereum/crypto"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"bytes"
	"github.com/ethereum/go-ethereum/rlp"
)

// TODO: This is a broken system that needs rearchitecting for multiple owners
// right now a message is marked read as soon as one owner gets it
// to send a message send the mailserver topic a NestedEnvelope
// destination should be the agent from the chaintip
// to get messages, the client comes up, requests messages
// if they are one of the owners, then the message is sent back

type Mailbox struct {
	storage storage.Storage
}

func NewMailbox(storage storage.Storage) *Mailbox {
	return &Mailbox{storage: storage}
}

func NewDbKey(hsh common.Hash) []byte {
	return hsh.Bytes()
}

func DestinationToBucket(dest []byte) []byte {
	return append([]byte("mailbox:"), dest...)
}

func (ms *Mailbox) Close() {
	ms.storage.Close()
}

func (ms *Mailbox) Archive(nestedEnvelope *mailserverpb.NestedEnvelope) error {
	key := NewDbKey(crypto.Keccak256Hash(nestedEnvelope.Envelope))
	ms.storage.CreateBucketIfNotExists(DestinationToBucket(nestedEnvelope.Destination))
	err := ms.storage.Set(DestinationToBucket(nestedEnvelope.Destination), key, nestedEnvelope.Envelope)
	if err != nil {
		log.Error(fmt.Sprintf("Writing to DB failed: %s", err))
		return fmt.Errorf("error setting: %v", err)
	}
	return nil
}

func (ms *Mailbox) Ack(destination, envHash []byte) error {
	key := NewDbKey(common.BytesToHash(envHash))
	return ms.storage.Delete(DestinationToBucket(destination), key)
}

func (ms *Mailbox) ForEach(destination []byte, iteratorFunc func(env *whisper.Envelope) error) error {
	return ms.storage.ForEach(DestinationToBucket(destination), func(_,v []byte) error {
		reader := bytes.NewReader(v)
		stream := rlp.NewStream(reader, 10 * 1024 * 1024) // 10MB limit
		env := &whisper.Envelope{}
		env.DecodeRLP(stream)
		return iteratorFunc(env)
	})
}

