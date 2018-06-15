// +build integration

package mailserver_test

import (
	"crypto/ecdsa"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/client/client"
	"github.com/quorumcontrol/qc3/client/wallet"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/mailserver"
	"github.com/quorumcontrol/qc3/mailserver/mailserverpb"
	"github.com/quorumcontrol/qc3/node"
	"github.com/quorumcontrol/qc3/notary"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/stretchr/testify/assert"
)

var blsHexKeys = []string{
	"0x0d8594abe3a33cc0210201ad33911defd69595990c1e54269c02a1f4b1487819",
	"0x107b54d1ccb67e0a89e523ceeacbbcc3aa2cdb3532920cf16734d0257235c890",
	"0x13d3744feea2a2167dbbdf25d38b394402deef1503e1561fef981e75c3aa85e1",
}

var ecdsaHexKeys = []string{
	"0xa3ec9da31e1daae7836f421dbdcc0636c82060cabb4d561b51772b4ba7f58510",
	"0xa3f35e4b79ecfc7cceb8f8b2db2335be9b59f7fd0e0db8b752e314be181654c9",
	"0xa1ee2cc2d5669c9e01db473c764db58036f84bf8eb061438dc468b6be1accc65",
}

var BlsSignKeys []*bls.SignKey
var EcdsaKeys []*ecdsa.PrivateKey

func init() {
	BlsSignKeys = make([]*bls.SignKey, len(blsHexKeys))
	EcdsaKeys = make([]*ecdsa.PrivateKey, len(ecdsaHexKeys))

	for i, hex := range blsHexKeys {
		BlsSignKeys[i] = bls.BytesToSignKey(hexutil.MustDecode(hex))
	}

	for i, hex := range ecdsaHexKeys {
		key, _ := crypto.ToECDSA(hexutil.MustDecode(hex))
		EcdsaKeys[i] = key
	}
}

type TestCluster struct {
	Nodes       []*node.WhisperNode
	Group       *notary.Group
	MailServers []*mailserver.MailServer
}

func NewDefaultTestCluster(t *testing.T) *TestCluster {
	keys := make([]*consensuspb.PublicKey, len(BlsSignKeys))
	for i, key := range BlsSignKeys {
		keys[i] = consensus.BlsKeyToPublicKey(key.MustVerKey())
	}
	group := notary.GroupFromPublicKeys(keys)

	nodes := make([]*node.WhisperNode, len(BlsSignKeys))
	mailservers := make([]*mailserver.MailServer, len(BlsSignKeys))
	for i, key := range BlsSignKeys {
		store := storage.NewMemStorage()
		chainStore := notary.NewChainStore("testTips", store)
		signer := notary.NewSigner(chainStore, group, key)
		nodes[i] = node.NewWhisperNode(signer, EcdsaKeys[i])

		mailbox := mailserver.NewMailbox(store)
		mailservers[i] = mailserver.NewMailServer(mailbox)
		mailservers[i].AttachToNode(nodes[i])
	}

	return &TestCluster{
		Nodes:       nodes,
		Group:       group,
		MailServers: mailservers,
	}
}

func (tc *TestCluster) Start() {
	for _, node := range tc.Nodes {
		node.Start()
	}
}

func (tc *TestCluster) Stop() {
	for _, node := range tc.Nodes {
		node.Stop()
	}
}

func TestMailserverIntegration(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

	destKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	cluster := NewDefaultTestCluster(t)
	cluster.Start()
	defer cluster.Stop()

	t.Log("creating client")
	memWallet := wallet.NewMemoryWallet("test")
	c := client.NewClient(cluster.Group, memWallet)
	c.Start()
	defer c.Stop()

	sentChat := &mailserverpb.ChatMessage{
		Message: []byte("ojai"),
	}

	err = c.SendMessage(mailserver.AlphaMailServerKey, &destKey.PublicKey, sentChat)
	assert.Nil(t, err)

	time.Sleep(time.Duration(5) * time.Second)

	count := 0
	cluster.MailServers[0].Mailbox.ForEach(crypto.FromECDSAPub(&destKey.PublicKey), func(env *whisper.Envelope) error {
		count++
		received, err := env.OpenAsymmetric(destKey)
		assert.Nil(t, err)
		assert.True(t, received.Validate())

		receivedAny := &types.Any{}
		err = proto.Unmarshal(received.Payload, receivedAny)
		assert.Nil(t, err)

		receivedChat, err := consensus.AnyToObj(receivedAny)
		assert.Nil(t, err)

		assert.Equal(t, sentChat, receivedChat)

		return nil
	})

	assert.Equal(t, count, 1)

	// now that we know the message is stored, let's ask for it back

	msgChan, err := c.GetMessages(destKey)
	assert.Nil(t, err)
	time.Sleep(time.Duration(2) * time.Second)
	assert.Equal(t, 1, len(msgChan))
}
