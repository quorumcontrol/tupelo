package walletrpc

import (
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/gossip3/client"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportExport(t *testing.T) {
	path := ".tmp/test"
	os.RemoveAll(path)
	os.MkdirAll(path, 0755)
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

	importedNew, err := sessNew.ImportChain(export, &adapters.Config{Adapter: "mock"})
	require.Nil(t, err)

	assert.Equal(t, chain.MustId(), importedNew.MustId())
}
