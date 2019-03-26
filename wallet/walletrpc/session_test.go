package walletrpc

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo-go-client/client"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportExport(t *testing.T) {
	path := ".tmp/test-import-export"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(path)
	ng := types.NewNotaryGroup("importtest")
	client := client.New(ng)
	sess, err := NewSession(path, "test-only", client)
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

	export, err := sess.ExportChain(chain.MustId())
	require.Nil(t, err)

	imported, err := sess.ImportChain(export, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	assert.Equal(t, chain.MustId(), imported.MustId())

	// Test importing to different wallet
	sessNew, err := NewSession(path, "test-only-new", client)
	require.Nil(t, err)

	err = sessNew.CreateWallet("test-new")
	require.Nil(t, err)

	err = sessNew.Start("test-new")
	require.Nil(t, err)

	defer sessNew.Stop()

	importedNew, err := sessNew.ImportChain(export, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	assert.Equal(t, chain.MustId(), importedNew.MustId())
}

func TestSendToken(t *testing.T) {
	path := ".tmp/test-send-token"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(path)
	ng := types.NewNotaryGroup("send-token-test")
	ts := testnotarygroup.NewTestSet(t, 1)
	signer := types.NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	syncer, err := actor.EmptyRootContext.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              signer,
		NotaryGroup:       ng,
		CommitStore:       storage.NewMemStorage(),
		CurrentStateStore: storage.NewMemStorage(),
	}), "tupelo-"+signer.ID)
	require.Nil(t, err)
	signer.Actor = syncer
	defer syncer.Poison()
	ng.AddSigner(signer)

	tupeloClient := client.New(ng)
	sess, err := NewSession(path, "send-token-test", tupeloClient)
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

	// TODO: Improve these assertions
	assert.NotEmpty(t, sendTokens.TransactionId)
	assert.NotEmpty(t, sendTokens.Leaves)
	assert.NotNil(t, sendTokens.Tip)
	assert.NotNil(t, sendTokens.Signature)
}
