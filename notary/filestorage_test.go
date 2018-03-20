package notary_test

import (
	"testing"
	"os"
	"github.com/quorumcontrol/qc3/notary"
	"path/filepath"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/stretchr/testify/assert"
)

func TestNewFileStorage(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")
	fs := notary.NewFileStorage(filepath.Join("testtmp", "testdb"))

	key,_ := crypto.GenerateKey()

	chain := chainFromEcdsaKey(t, &key.PublicKey)

	tip := consensus.ChainToTip(chain)

	err := fs.Set(chain.Id, tip)
	assert.Nil(t, err)

	retTip,err := fs.Get(chain.Id)
	assert.Nil(t,err)
	assert.Equal(t, retTip, tip)
}
