package walletrpc

import (
	"context"
	"encoding/base64"
	"os"
	"testing"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/gogo/protobuf/proto"
	"github.com/quorumcontrol/messages/build/go/transactions"
	"github.com/quorumcontrol/tupelo/storage"

	"github.com/quorumcontrol/chaintree/safewrap"

	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/testnotarygroup"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
)

func TestImportExport(t *testing.T) {
	path := ".tmp/test-import-export"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(path)
	ng := types.NewNotaryGroup("importtest")
	ts := testnotarygroup.NewTestSet(t, 1)
	signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
	pubSubSystem := remote.NewSimulatedPubSub()

	syncer, err := actor.EmptyRootContext.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              signer,
		NotaryGroup:       ng,
		CurrentStateStore: storage.NewDefaultMemory(),
		PubSubSystem:      pubSubSystem,
	}), "tupelo-"+signer.ID)
	require.Nil(t, err)
	signer.Actor = syncer
	defer actor.EmptyRootContext.Poison(syncer)
	ng.AddSigner(signer)

	sess, err := NewSession(path, "test-only", ng, pubSubSystem)
	require.Nil(t, err)

	err = sess.CreateWallet("test")
	require.Nil(t, err)

	err = sess.Start("test")
	require.Nil(t, err)

	defer sess.Stop()

	key, err := sess.GenerateKey()
	require.Nil(t, err)

	addr := crypto.PubkeyToAddress(key.PublicKey).String()

	chain, err := sess.CreateChain(addr, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}
	_, err = sess.SetData(chain.MustId(), addr, "test", sw.WrapObject("worked").RawData())
	require.Nil(t, err)

	export, err := sess.ExportChain(chain.MustId())
	require.Nil(t, err)

	imported, err := sess.ImportChain(export, true, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	assert.Equal(t, chain.MustId(), imported.MustId())

	// Test importing to different wallet
	sessNew, err := NewSession(path, "test-only-new", ng, pubSubSystem)
	require.Nil(t, err)

	err = sessNew.CreateWallet("test-new")
	require.Nil(t, err)

	err = sessNew.Start("test-new")
	require.Nil(t, err)

	defer sessNew.Stop()

	importedNew, err := sessNew.ImportChain(export, true, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	assert.Equal(t, chain.MustId(), importedNew.MustId())

	// Test importing to a wallet with a different notary group
	otherNg := types.NewNotaryGroup("importtest-other")
	sessOther, err := NewSession(path, "other-import-export", otherNg, pubSubSystem)
	require.Nil(t, err)

	err = sessOther.CreateWallet("test-other")
	require.Nil(t, err)

	err = sessOther.Start("test-other")
	require.Nil(t, err)

	defer sessOther.Stop()

	_, err = sessOther.ImportChain(export, true, &adapters.Config{Adapter: "mock"})
	require.NotNil(t, err)

	importedUnverified, err := sessOther.ImportChain(export, false, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)
	assert.Equal(t, chain.MustId(), importedUnverified.MustId())
}

func TestSendAndReceiveToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	path := ".tmp/test-send-token"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(path)
	ng := types.NewNotaryGroup("send-token-test")
	ts := testnotarygroup.NewTestSet(t, 1)
	signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
	pubSubSystem := remote.NewSimulatedPubSub()

	syncer, err := actor.EmptyRootContext.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              signer,
		NotaryGroup:       ng,
		CurrentStateStore: storage.NewDefaultMemory(),
		PubSubSystem:      pubSubSystem,
	}), "tupelo-"+signer.ID)
	require.Nil(t, err)
	signer.Actor = syncer
	defer actor.EmptyRootContext.Poison(syncer)
	ng.AddSigner(signer)

	sess, err := NewSession(path, "send-token-test", ng, pubSubSystem)
	require.Nil(t, err)

	err = sess.CreateWallet("test")
	require.Nil(t, err)

	err = sess.Start("test")
	require.Nil(t, err)

	defer sess.Stop()

	key, err := sess.GenerateKey()
	require.Nil(t, err)

	addr := crypto.PubkeyToAddress(key.PublicKey).String()

	chain, err := sess.CreateChain(addr, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	destKey, err := sess.GenerateKey()
	require.Nil(t, err)

	destAddr := crypto.PubkeyToAddress(destKey.PublicKey).String()

	destChain, err := sess.CreateChain(destAddr, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	_, err = sess.EstablishToken(chain.MustId(), addr, "test-token", 1000000)
	require.Nil(t, err)

	_, err = sess.MintToken(chain.MustId(), addr, "test-token", 10)
	require.Nil(t, err)

	sendTokens, err := sess.SendToken(chain.MustId(), addr, "test-token", destChain.MustId(), 5)
	require.Nil(t, err)
	decodedSendTokens, err := base64.StdEncoding.DecodeString(sendTokens)
	require.Nil(t, err)
	unmarshalledSendTokens := &transactions.TokenPayload{}
	err = proto.Unmarshal(decodedSendTokens, unmarshalledSendTokens)
	require.Nil(t, err)

	// TODO: Improve these assertions
	assert.NotEmpty(t, unmarshalledSendTokens.TransactionId)
	assert.NotEmpty(t, unmarshalledSendTokens.Leaves)
	assert.NotNil(t, unmarshalledSendTokens.Tip)
	assert.NotNil(t, unmarshalledSendTokens.Signature)

	receiveTokensTip, err := sess.ReceiveToken(destChain.MustId(), destAddr, sendTokens)
	require.Nil(t, err)
	require.NotNil(t, receiveTokensTip)

	destChain, err = sess.GetChain(destChain.MustId())
	require.Nil(t, err)

	destChainTree, err := destChain.ChainTree.At(ctx, receiveTokensTip)
	require.Nil(t, err)

	destTree, err := destChainTree.Tree(ctx)
	require.Nil(t, err)

	senderTree, err := chain.ChainTree.Tree(ctx)
	require.Nil(t, err)
	canonicalTokenName, err := consensus.CanonicalTokenName(senderTree, chain.MustId(), "test-token", true)
	require.Nil(t, err)

	ledger := consensus.NewTreeLedger(destTree, canonicalTokenName)

	balance, err := ledger.Balance()
	require.Nil(t, err)

	assert.Equal(t, uint64(5), balance)
}

func TestGetTip(t *testing.T) {
	path := ".tmp/get-tip-test"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(path)

	ng := types.NewNotaryGroup("get-tip-test")
	ts := testnotarygroup.NewTestSet(t, 1)
	signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
	pubSubSystem := remote.NewSimulatedPubSub()

	syncer, err := actor.EmptyRootContext.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              signer,
		NotaryGroup:       ng,
		CurrentStateStore: storage.NewDefaultMemory(),
		PubSubSystem:      pubSubSystem,
	}), "tupelo-"+signer.ID)
	require.Nil(t, err)
	signer.Actor = syncer
	defer actor.EmptyRootContext.Poison(syncer)
	ng.AddSigner(signer)

	sess, err := NewSession(path, "get-tip-test", ng, pubSubSystem)
	require.Nil(t, err)

	err = sess.CreateWallet("test")
	require.Nil(t, err)

	err = sess.Start("test")
	require.Nil(t, err)

	defer sess.Stop()

	key, err := sess.GenerateKey()
	require.Nil(t, err)

	addr := crypto.PubkeyToAddress(key.PublicKey).String()

	chain, err := sess.CreateChain(addr, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}

	newTip, err := sess.SetData(chain.MustId(), addr, "test", sw.WrapObject("worked").RawData())
	require.Nil(t, err)

	getTipResp, err := sess.GetTip(chain.MustId())
	require.Nil(t, err)

	require.Equal(t, newTip, getTipResp)
}

func testGetTokenBalance(t *testing.T, sess *RPCSession, chain *consensus.SignedChainTree) {
	bal, err := sess.GetTokenBalance(chain.MustId(), "test-token")
	require.Nil(t, err)
	require.Equal(t, uint64(10), bal)
}

func testGetNonExistentTokenBalance(t *testing.T, sess *RPCSession,
	chain *consensus.SignedChainTree) {
	bal, err := sess.GetTokenBalance(chain.MustId(), "non-existent-token")
	require.NotNil(t, err)
	require.Equal(t, uint64(0), bal)
}

func TestTokens(t *testing.T) {
	testCases := []struct {
		name   string
		testFn func(*testing.T, *RPCSession, *consensus.SignedChainTree)
	}{
		{"get existing token balance", testGetTokenBalance},
		{"get non-existent token balance", testGetNonExistentTokenBalance},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := ".tmp/test-tokens"
			err := os.RemoveAll(path)
			require.Nil(t, err)
			err = os.MkdirAll(path, 0755)
			require.Nil(t, err)
			defer os.RemoveAll(path)

			ng := types.NewNotaryGroup("test-tokens")
			ts := testnotarygroup.NewTestSet(t, 1)
			signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
			pubSubSystem := remote.NewSimulatedPubSub()

			syncer, err := actor.EmptyRootContext.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
				Self:              signer,
				NotaryGroup:       ng,
				CurrentStateStore: storage.NewDefaultMemory(),
				PubSubSystem:      pubSubSystem,
			}), "tupelo-"+signer.ID)
			require.Nil(t, err)
			signer.Actor = syncer
			defer actor.EmptyRootContext.Poison(syncer)
			ng.AddSigner(signer)

			sess, err := NewSession(path, "test-tokens", ng, pubSubSystem)
			require.Nil(t, err)

			err = sess.CreateWallet("test")
			require.Nil(t, err)

			err = sess.Start("test")
			require.Nil(t, err)

			defer sess.Stop()

			key, err := sess.GenerateKey()
			require.Nil(t, err)
			addr := crypto.PubkeyToAddress(key.PublicKey).String()
			chain, err := sess.CreateChain(addr, &adapters.Config{Adapter: "mock"})
			require.Nil(t, err)

			_, err = sess.EstablishToken(chain.MustId(), addr, "test-token", 1000000)
			require.Nil(t, err)

			_, err = sess.MintToken(chain.MustId(), addr, "test-token", 10)
			require.Nil(t, err)

			tc.testFn(t, sess, chain)
		})
	}
}
