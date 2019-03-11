package wallet

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	cbornode "github.com/ipfs/go-ipld-cbor"
	ipfsCmds "github.com/ipsn/go-ipfs/commands"
	ipfsCore "github.com/ipsn/go-ipfs/core"
	ipfsCoreHttp "github.com/ipsn/go-ipfs/core/corehttp"
	ipfsMock "github.com/ipsn/go-ipfs/core/mock"
	ipfsConfig "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-ipfs-config"
	ma "github.com/ipsn/go-ipfs/gxlibs/github.com/multiformats/go-multiaddr"
	ipfsPluginLoader "github.com/ipsn/go-ipfs/plugin/loader"
	ipfsFsRepo "github.com/ipsn/go-ipfs/repo/fsrepo"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/consensus"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockAdapterWallet(t *testing.T) {
	storageConfig := &adapters.Config{Adapter: "mock"}
	SubtestAll(t, storageConfig)
}

func TestBadgerWallet(t *testing.T) {
	err := os.RemoveAll("testtmp")
	require.Nil(t, err)
	err = os.MkdirAll("testtmp", 0700)
	require.Nil(t, err)
	defer os.RemoveAll("testtmp")

	storageConfig := &adapters.Config{
		Adapter: "badger",
		Arguments: map[string]interface{}{
			"path": "testtmp/adapter",
		},
	}

	SubtestAll(t, storageConfig)
}

func TestIpldHttpWallet(t *testing.T) {
	node, err := ipfsMock.NewMockNode()
	require.Nil(t, err)
	defer node.Close()

	freePort, err := getFreePort()
	require.Nil(t, err)

	apiMaddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", freePort))
	require.Nil(t, err)

	cfg, err := node.Repo.Config()
	require.Nil(t, err)

	cfg.Addresses.API = []string{apiMaddr.String()}

	cmdContext := ipfsCmds.Context{
		Online:     true,
		ConfigRoot: "/tmp/.mockipfsconfig",
		ReqLog:     &ipfsCmds.ReqLog{},
		LoadConfig: func(path string) (*ipfsConfig.Config, error) {
			return cfg, nil
		},
		ConstructNode: func() (*ipfsCore.IpfsNode, error) {
			return node, nil
		},
	}

	go func() {
		err := ipfsCoreHttp.ListenAndServe(node, apiMaddr.String(), []ipfsCoreHttp.ServeOption{ipfsCoreHttp.CommandsOption(cmdContext)}...)
		require.Nil(t, err)
	}()

	time.Sleep(50 * time.Millisecond)

	SubtestAll(t, &adapters.Config{
		Adapter: "ipld",
		Arguments: map[string]interface{}{
			"address": apiMaddr.String(),
		},
	})
}

func TestIpldWallet(t *testing.T) {
	err := os.RemoveAll("testtmp")
	require.Nil(t, err)
	err = os.MkdirAll("testtmp", 0700)
	require.Nil(t, err)
	defer os.RemoveAll("testtmp")

	storageConfig := &adapters.Config{
		Adapter: "ipld",
		Arguments: map[string]interface{}{
			"path": "testtmp/ipld",
		},
	}

	plugins, err := ipfsPluginLoader.NewPluginLoader("")
	require.Nil(t, err)
	err = plugins.Initialize()
	require.Nil(t, err)
	err = plugins.Inject()
	require.Nil(t, err)

	conf, err := ipfsConfig.Init(os.Stdout, 2048)
	require.Nil(t, err, "error initializing IPFS")

	for _, profile := range []string{"server", "badgerds"} {
		transformer, ok := ipfsConfig.Profiles[profile]
		require.True(t, ok, "error fetching IPFS profile")

		err := transformer.Transform(conf)
		require.Nil(t, err, "error transforming IPFS profile")
	}

	err = ipfsFsRepo.Init("testtmp/ipld", conf)
	require.Nil(t, err, "error initializing IPFS repo")

	SubtestAll(t, storageConfig)
}

func SubtestAll(t *testing.T, storageConfig *adapters.Config) {
	SubtestWallet_GetChain(t, storageConfig)
	SubtestWallet_SaveChain(t, storageConfig)
	SubtestWallet_GetChainIds(t, storageConfig)
	SubtestWallet_ChainExists(t, storageConfig)
	SubtestWallet_GetKey(t, storageConfig)
	SubtestWallet_ChainExists(t, storageConfig)
}

func SubtestWallet_GetChain(t *testing.T, storageConfig *adapters.Config) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	key, err := w.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()

	newChain, err := w.CreateChain(keyAddr, storageConfig)
	// require.Nil(t, err)

	err = w.SaveChain(newChain)
	require.Nil(t, err)

	savedChain, err := w.GetChain(newChain.MustId())
	assert.Nil(t, err)

	assert.Equal(t, newChain.Tip(), savedChain.Tip())

	origNodes, err := newChain.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	savedNodes, err := savedChain.ChainTree.Dag.Nodes()
	require.Nil(t, err)
	assert.Equal(t, len(origNodes), len(savedNodes))

	origCids := make([]string, len(origNodes))
	newCids := make([]string, len(savedNodes))

	for i, node := range origNodes {
		origCids[i] = node.Cid().String()
	}

	for i, node := range savedNodes {
		newCids[i] = node.Cid().String()
	}
	sort.Strings(origCids)
	sort.Strings(newCids)

	assert.Equal(t, origCids, newCids)
}

func SubtestWallet_SaveChain(t *testing.T, storageConfig *adapters.Config) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	key, err := w.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()

	signedTree, err := w.CreateChain(keyAddr, storageConfig)
	require.Nil(t, err)

	err = w.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err := w.GetChain(signedTree.MustId())
	assert.Nil(t, err)
	signedNodes, err := signedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	savedNodes, err := savedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)
	assert.Equal(t, len(signedNodes), len(savedNodes))

	hsh := crypto.Keccak256([]byte("hi"))

	ecdsaSig, _ := crypto.Sign(hsh, key)

	sig := &consensus.Signature{
		Type:      consensus.KeyTypeSecp256k1,
		Signature: ecdsaSig,
	}

	signedTree.Signatures = consensus.SignatureMap{
		"id": *sig,
	}

	err = w.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err = w.GetChain(signedTree.MustId())
	assert.Nil(t, err)

	savedNodes, err = savedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	assert.Equal(t, len(signedNodes), len(savedNodes))

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: nil,
			Transactions: []*chaintree.Transaction{
				{
					Type: consensus.TransactionTypeSetData,
					Payload: &consensus.SetDataPayload{
						Path:  "something",
						Value: "hi",
					},
				},
			},
		},
	}

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, key)
	require.Nil(t, err)

	newTree := signedTree.ChainTree.Dag.WithNewTip(signedTree.ChainTree.Dag.Tip)

	unmarshaledRoot, err := newTree.Get(newTree.Tip)
	require.Nil(t, err)
	require.NotNil(t, unmarshaledRoot)

	root := &chaintree.RootNode{}

	err = cbornode.DecodeInto(unmarshaledRoot.RawData(), root)
	require.Nil(t, err)

	require.NotNil(t, root.Tree)

	newTree.Tip = *root.Tree

	_, err = newTree.Set(strings.Split("something", "/"), "hi")
	require.Nil(t, err)
	_, err = signedTree.ChainTree.Dag.SetAsLink([]string{chaintree.TreeLabel}, newTree)
	// require.Nil(t, err)

	chainNode, err := signedTree.ChainTree.Dag.Get(*root.Chain)
	require.Nil(t, err)
	chainData, err := nodestore.CborNodeToObj(chainNode)
	chainMap := chainData.(map[string]interface{})
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}

	wrappedBlock := sw.WrapObject(blockWithHeaders)
	require.Nil(t, sw.Err)

	chainMap["end"] = wrappedBlock.Cid()
	newChainNode := sw.WrapObject(chainMap)

	err = signedTree.ChainTree.Dag.AddNodes(wrappedBlock)
	require.Nil(t, err)
	_, err = signedTree.ChainTree.Dag.Update([]string{chaintree.ChainLabel}, newChainNode)
	// require.Nil(t, err)

	t.Log(signedTree.ChainTree.Dag.Dump())

	err = w.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err = w.GetChain(signedTree.MustId())
	require.Nil(t, err)
	savedNodes, err = savedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	assert.Equal(t, len(signedNodes), len(savedNodes))

}

func SubtestWallet_GetChainIds(t *testing.T, storageConfig *adapters.Config) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	createdChains := make([]string, 3)

	for i := 0; i < 3; i++ {
		key, err := w.GenerateKey()
		require.Nil(t, err)

		keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()
		signedTree, err := w.CreateChain(keyAddr, storageConfig)
		createdChains[i] = signedTree.MustId()
		require.Nil(t, err)
	}

	ids, err := w.GetChainIds()
	assert.Nil(t, err)

	assert.ElementsMatch(t, createdChains, ids)
}

func SubtestWallet_GetKey(t *testing.T, storageConfig *adapters.Config) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	key, err := w.GenerateKey()
	assert.Nil(t, err)

	retKey, err := w.GetKey(crypto.PubkeyToAddress(key.PublicKey).String())
	assert.Nil(t, err)
	assert.Equal(t, retKey, key)
}

func SubtestWallet_ChainExists(t *testing.T, storageConfig *adapters.Config) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	key, err := w.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()
	chainId := consensus.EcdsaPubkeyToDid(key.PublicKey)

	assert.False(t, w.ChainExistsForKey(keyAddr))
	assert.False(t, w.ChainExists(chainId))

	signedTree, err := w.CreateChain(keyAddr, storageConfig)
	require.Nil(t, err)

	assert.True(t, w.ChainExistsForKey(keyAddr))
	assert.True(t, w.ChainExists(signedTree.MustId()))
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
