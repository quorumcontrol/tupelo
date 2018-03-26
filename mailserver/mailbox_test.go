package mailserver_test

import (
	"testing"
	"github.com/ethereum/go-ethereum/crypto"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/stretchr/testify/assert"
	"os"
	"github.com/quorumcontrol/qc3/mailserver"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/quorumcontrol/qc3/mailserver/mailserverpb"
	"path/filepath"
	"github.com/ethereum/go-ethereum/rlp"
)

var aliceKey,_ = crypto.GenerateKey()
var aliceAddr = crypto.PubkeyToAddress(aliceKey.PublicKey)

var bobKey,_ = crypto.GenerateKey()
var bobAddr = crypto.PubkeyToAddress(bobKey.PublicKey)

var topic = []byte("topic")
var commonPayload = []byte("payload")

func validEnvelope(t *testing.T) *whisper.Envelope {
	params := &whisper.MessageParams{
		TTL: 60,
		Dst: &bobKey.PublicKey,
		Src: aliceKey,
		Topic: whisper.BytesToTopic(topic),
		PoW: .002,  // TODO: what are the right settings for PoW?
		WorkTime: 2,
		Payload: commonPayload,
	}
	msg,err := whisper.NewSentMessage(params)
	assert.Nil(t, err)
	env,err := msg.Wrap(params)
	assert.Nil(t,err)
	return env
}

func defaultMailServer(t *testing.T) *mailserver.Mailbox {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	bolt := storage.NewBoltStorage(filepath.Join("testtmp", "testdb"))
	return mailserver.NewMailbox(bolt)

}

func cleanup(ms *mailserver.Mailbox) {
	ms.Close()
	os.RemoveAll("testtmp")
}

func TestMailServer_Archive(t *testing.T) {
	ms := defaultMailServer(t)
	defer cleanup(ms)

	dst := []byte("nomatter")

	testEnv := validEnvelope(t)
	bytes,err := rlp.EncodeToBytes(testEnv)
	assert.Nil(t,err)

	err = ms.Archive(&mailserverpb.NestedEnvelope{
		Destination: dst,
		Envelope: bytes,
	})

	assert.Nil(t, err)

	ms.ForEach(dst, func(env *whisper.Envelope) error {
		assert.Equal(t, env.Hash(),testEnv.Hash())
		msg,err := env.OpenAsymmetric(bobKey)
		assert.Nil(t, err)
		assert.True(t, msg.Validate())
		assert.Equal(t, commonPayload, msg.Payload)
		return nil
	})
}

func TestMailServer_Ack(t *testing.T) {
	ms := defaultMailServer(t)
	defer cleanup(ms)

	dst := []byte("nomatter")

	testEnv := validEnvelope(t)

	bytes,err := rlp.EncodeToBytes(testEnv)
	assert.Nil(t,err)

	err = ms.Archive(&mailserverpb.NestedEnvelope{
		Destination: dst,
		Envelope: bytes,
	})

	assert.Nil(t, err)

	err = ms.Ack(dst, crypto.Keccak256(bytes))
	assert.Nil(t,err)

	count := 0
	ms.ForEach(dst, func(env *whisper.Envelope) error {
		count++
		return nil
	})

	assert.Equal(t, count, 0)
}
