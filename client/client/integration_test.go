// +build integration

package client_test

import (
	"crypto/ecdsa"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/gogo/protobuf/types"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/client/client"
	"github.com/quorumcontrol/qc3/client/wallet"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/mailserver"
	"github.com/quorumcontrol/qc3/node"
	"github.com/quorumcontrol/qc3/notary"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/stretchr/testify/assert"
)

var blsHexKeys = []string{
	"0x1cbf9876aab27c7261ba8554fbb60b88b9a5e4ce9fe08cd2a368d1b3558045e1",
	"0x10aa5d86e4b4b79cfa1363b7930702e3dcee34e17fb51b3ddd4c58d752ebcb88",
	"0x0bb749fe4f2b269f8891e15f04225a68088eb2db489101d7cd4d3c5cd35f93fc",
	"0x22c8be919bc9d220b5dd0f7c8fa5781688ef494894969813172b6ffa689909f2",
	"0x1144c42cbaccd196e0811501a2a8f9850f8a37aab085e461507f3594d3ea7523",
}

var ecdsaHexKeys = []string{
	"0x29820eed8a4d15258a614b257542e905a12d4dee577f15b3daa38a5171b4f998",
	"0x7666c571635732722f036befa11f931ffa67c3bd337df63ee17aadc6a4d95ce0",
	"0xcd98169b4540f39e9266efad60498832fa8f713a1ed7d720ff8b4ec864acd4c7",
	"0x57dc7c4ae06b9a0d8dfaa4236f42bc33d3184b4ef65df7b32608c0fbf2410fde",
	"0x4d8a392f682359ec6c0071034d65c52667c54adc130a2e7648e149fd4cdb144b",
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

func TestFullIntegration(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

	cluster := NewDefaultTestCluster(t)
	cluster.Start()
	defer cluster.Stop()

	log.Debug("creating client")
	memWallet := wallet.NewMemoryWallet("test")
	c := client.NewClient(cluster.Group, memWallet)
	c.Start()
	defer c.Stop()

	newChainKey, _ := crypto.GenerateKey()
	log.Debug("creating chain")
	newChain, err := c.CreateChain(newChainKey)
	assert.Nil(t, err)

	walletChain, err := c.Wallet.GetChain(newChain.Id)
	assert.Nil(t, err)

	isSigned, err := cluster.Group.IsBlockSigned(walletChain.Blocks[len(walletChain.Blocks)-1])
	assert.Nil(t, err)

	assert.True(t, isSigned)

	// now test that we can get the proper chain tip back
	log.Debug("getting chain tip")

	tipChan, err := c.GetTip(newChain.Id)
	assert.Nil(t, err)
	assert.NotNil(t, tipChan)

	tip := <-tipChan
	hsh, err := consensus.BlockToHash(newChain.Blocks[0])
	assert.Nil(t, err)
	assert.NotNil(t, tip)

	assert.Equal(t, tip.LastHash, hsh.Bytes())

	// test minting coins
	log.Debug("minting coins")

	c.SetCurrentIdentity(newChainKey)

	doneChan, err := c.MintCoin(newChain.Id, "cat_coin", 100)
	assert.Nil(t, err)
	assert.True(t, <-doneChan)

	// sending coins doesn't error

	log.Debug("creating receive chain")

	receiveChainKey, _ := crypto.GenerateKey()
	receiveChain, err := c.CreateChain(receiveChainKey)
	assert.Nil(t, err)

	log.Debug("sending coin")
	err = c.SendCoin(newChain.Id, receiveChain.Id, "cat_coin", 1)
	assert.Nil(t, err)

	balances, err := c.Wallet.Balances(newChain.Id)
	assert.Nil(t, err)
	assert.Equal(t, uint64(99), balances[newChain.Id+":cat_coin"])

	// now let's get that coin
	log.Debug("receiving coin")
	time.Sleep(1 * time.Second)
	c.SetCurrentIdentity(receiveChainKey)

	messageChan, err := c.GetMessages(receiveChainKey)
	assert.Nil(t, err)

	anyMsg := <-messageChan
	assert.NotNil(t, anyMsg)

	sendCoinMessage, err := consensus.AnyToObj(anyMsg.(*types.Any))
	assert.Nil(t, err)

	log.Debug("processing send coin message to receive the coin")
	done, err := c.ProcessSendCoinMessage(sendCoinMessage.(*consensuspb.SendCoinMessage))
	assert.Nil(t, err)
	assert.True(t, <-done)

	balances, err = c.Wallet.Balances(receiveChain.Id)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), balances[newChain.Id+":cat_coin"])

}
