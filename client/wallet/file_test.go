package wallet_test

import (
	"testing"
	"os"
	"github.com/quorumcontrol/qc3/client/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestFileWallet_GetKey(t *testing.T) {

	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := wallet.NewFileWallet("password", "testtmp/filewallet")
	defer fw.Close()

	key,err := fw.GenerateKey()
	assert.Nil(t, err)

	retKey,err := fw.GetKey(crypto.PubkeyToAddress(key.PublicKey).String())

	assert.Equal(t, retKey, key)

}
