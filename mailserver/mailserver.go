package mailserver

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/common"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/quorumcontrol/qc3/mailserver/mailserverpb"
	"github.com/ethereum/go-ethereum/crypto"
)

type MailServer struct {
	storage storage.Storage
}

func NewDbKey(hsh common.Hash) []byte {
	return hsh.Bytes()
}

func (ms *MailServer) Archive(nestedEnvelope *mailserverpb.NestedEnvelope) error {
	key := NewDbKey(crypto.Keccak256Hash(nestedEnvelope.Envelope))
	ms.storage.CreateBucketIfNotExists(nestedEnvelope.Destination)
	err := ms.storage.Set(nestedEnvelope.Destination, key, nestedEnvelope.Envelope)
	if err != nil {
		log.Error(fmt.Sprintf("Writing to DB failed: %s", err))
		return fmt.Errorf("error setting: %v", err)
	}
	return nil
}

func (ms *MailServer) Ack(destination, envHash []byte) error {
	key := NewDbKey(common.BytesToHash(envHash))
	return ms.storage.Delete(destination, key)
}
