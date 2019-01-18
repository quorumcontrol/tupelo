package wallet

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"strings"

	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
)

type UnlockInexistentWalletError struct {
	path string
}

func (e *UnlockInexistentWalletError) Error() string {
	return fmt.Sprintf("Can't unlock wallet at path '%s'. It does not exist. Create it first", e.path)
}

type CreateExistingWalletError struct {
	path string
}

func (e *CreateExistingWalletError) Error() string {
	return fmt.Sprintf("Can't create wallet at path '%s'. Another wallet already exists at the same path.", e.path)
}

// just make sure that implementation conforms to the interface
var _ consensus.Wallet = (*FileWallet)(nil)

type FileWallet struct {
	path   string
	wallet *Wallet
}

// isExists checks if the wallet specified by `path` already exists.
func (fw *FileWallet) isExists() bool {
	_, err := os.Stat(fw.path)
	return !os.IsNotExist(err)
}

func NewFileWallet(path string) *FileWallet {
	return &FileWallet{
		path: path,
	}
}

// CreateIfNotExists creates a new wallet at the path specified by `fw` if one
// hasn't already been created at that path.
func (fw *FileWallet) CreateIfNotExists(passphrase string) {
	store, err := storage.NewBadgerStorage(fw.path)
	if err != nil {
		panic(fmt.Sprintf("error creating badger store: %v", err))
	}
	encryptedStore := storage.NewEncryptedStore(store)
	encryptedStore.Unlock(passphrase)
	fw.wallet = NewWallet(&WalletConfig{Storage: encryptedStore})
}

// Create creates a new wallet at the path specified by `fw`. It returns an
// error if a wallet already exists at that path, and nil otherwise.
func (fw *FileWallet) Create(passphrase string) error {
	if fw.isExists() {
		return &CreateExistingWalletError{
			path: fw.path,
		}
	}

	fw.CreateIfNotExists(passphrase)

	return nil
}

// Unlock opens a pre-existing wallet after first validating `passphrase`
// against that wallet. It returns an error if the passphrase is not correct or
// the wallet doesn't already exist, and nil otherwise.
func (fw *FileWallet) Unlock(passphrase string) error {
	if !fw.isExists() {
		return &UnlockInexistentWalletError{
			path: fw.path,
		}
	}

	fw.CreateIfNotExists(passphrase)
	return nil
}

func (fw *FileWallet) Close() {
	if fw.wallet != nil {
		fw.wallet.Close()
	}
}

func (fw *FileWallet) GetTip(chainId string) ([]byte, error) {
	return fw.wallet.GetTip(chainId)
}

func (fw *FileWallet) GetChain(id string) (*consensus.SignedChainTree, error) {
	return fw.wallet.GetChain(id)
}

func (fw *FileWallet) ChainExists(chainId string) bool {
	return fw.wallet.ChainExists(chainId)
}

func (fw *FileWallet) SaveChain(signedChain *consensus.SignedChainTree) error {
	chainId, err := signedChain.Id()
	if err != nil {
		return fmt.Errorf("error getting signedChain id: %v", err)
	}

	if !fw.ChainExists(chainId) {
		fw.wallet.ConfigureChainStorage(chainId, &StorageAdapterConfig{
			Adapter: "badger",
			Arguments: map[string]interface{}{
				"path": strings.TrimRight(fw.path, "/") + "-" + chainId,
			},
		})
	}

	return fw.wallet.SaveChain(signedChain)
}

func (fw *FileWallet) GetChainIds() ([]string, error) {
	return fw.wallet.GetChainIds()
}

func (fw *FileWallet) GetKey(addr string) (*ecdsa.PrivateKey, error) {
	return fw.wallet.GetKey(addr)
}

func (fw *FileWallet) GenerateKey() (*ecdsa.PrivateKey, error) {
	return fw.wallet.GenerateKey()
}

func (fw *FileWallet) ListKeys() ([]string, error) {
	return fw.wallet.ListKeys()
}
