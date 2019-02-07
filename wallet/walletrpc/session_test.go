package walletrpc

import (
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip2client"
	"github.com/quorumcontrol/tupelo/p2p"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportExport(t *testing.T) {
	path := ".tmp/test"
	os.RemoveAll(path)
	os.MkdirAll(path, 0755)
	defer os.RemoveAll(path)
	ns := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	ng := consensus.NewNotaryGroup("importtest", ns)
	client := gossip2client.NewGossipClient(ng, p2p.BootstrapNodes())
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

	chain, err := sess.CreateChain(addr)
	require.Nil(t, err)

	export, err := sess.ExportChain(chain.MustId())
	require.Nil(t, err)

	imported, err := sess.ImportChain(export)
	require.Nil(t, err)

	assert.Equal(t, chain.MustId(), imported.MustId())

}
