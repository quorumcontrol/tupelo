package walletrpc

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/gob"
	"errors"
	fmt "fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/crypto"
	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	gossip3client "github.com/quorumcontrol/tupelo/gossip3/client"
	gossip3types "github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/wallet"
)

type RPCSession struct {
	client    *gossip3client.Client
	wallet    *wallet.FileWallet
	isStarted bool
}

type ExistingChainError struct {
	publicKey *ecdsa.PublicKey
}

func (e ExistingChainError) Error() string {
	keyAddr := crypto.PubkeyToAddress(*e.publicKey).String()
	return fmt.Sprintf("A chain tree for public key %v has already been created.", keyAddr)
}

func walletPath(parent string, name string) string {
	return filepath.Join(parent, name+"-wallet")
}

var StoppedError = errors.New("Unstarted wallet session")

func (rpcs *RPCSession) IsStopped() bool {
	return !rpcs.isStarted
}

type NilTipError struct {
	chainId     string
	notaryGroup *gossip3types.NotaryGroup
}

func (e *NilTipError) Error() string {
	return fmt.Sprintf("Chain tree with id %v is not known to the notary group %v", e.chainId, e.notaryGroup)
}

func NewSession(storagePath string, walletName string, gossipClient *gossip3client.Client) (*RPCSession, error) {
	path := walletPath(storagePath, walletName)

	fileWallet := wallet.NewFileWallet(path)

	return &RPCSession{
		client:    gossipClient,
		wallet:    fileWallet,
		isStarted: false,
	}, nil
}

func (rpcs *RPCSession) CreateWallet(passPhrase string) error {
	defer rpcs.wallet.Close()
	return rpcs.wallet.Create(passPhrase)
}

func (rpcs *RPCSession) Start(passPhrase string) error {
	if !rpcs.isStarted {
		unlockErr := rpcs.wallet.Unlock(passPhrase)
		if unlockErr != nil {
			return unlockErr
		}
		rpcs.isStarted = true
	}
	return nil
}

func (rpcs *RPCSession) Stop() {
	if !rpcs.IsStopped() {
		rpcs.wallet.Close()
		rpcs.isStarted = false
	}
}

func (rpcs *RPCSession) GenerateKey() (*ecdsa.PrivateKey, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	return rpcs.wallet.GenerateKey()
}

func (rpcs *RPCSession) ListKeys() ([]string, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	return rpcs.wallet.ListKeys()
}

func (rpcs *RPCSession) getKey(keyAddr string) (*ecdsa.PrivateKey, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	return rpcs.wallet.GetKey(keyAddr)
}

func (rpcs *RPCSession) chainExists(key ecdsa.PublicKey) bool {
	chainId := consensus.EcdsaPubkeyToDid(key)
	return rpcs.wallet.ChainExists(chainId)
}

func (rpcs *RPCSession) CreateChain(keyAddr string) (*consensus.SignedChainTree, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	key, err := rpcs.getKey(keyAddr)
	if err != nil {
		return nil, fmt.Errorf("Error getting key: %v", err)
	}

	if rpcs.chainExists(key.PublicKey) {
		return nil, ExistingChainError{publicKey: &key.PublicKey}
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	chain, err := consensus.NewSignedChainTree(key.PublicKey, nodeStore)
	if err != nil {
		return nil, err
	}

	rpcs.wallet.SaveChain(chain)
	return chain, nil
}

func serializeChain(chain *consensus.SignedChainTree) (string, error) {
	var chainBuffer bytes.Buffer
	encoder := gob.NewEncoder(&chainBuffer)
	err := encoder.Encode(chain)
	if err != nil {
		return "", fmt.Errorf("error encoding chain: %v", err)
	}

	return base64.StdEncoding.EncodeToString(chainBuffer.Bytes()), nil
}

func (rpcs *RPCSession) ExportChain(chainId string) (string, error) {
	if rpcs.IsStopped() {
		return "", StoppedError
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return "", err
	}

	serializedChain, err := serializeChain(chain)
	if err != nil {
		return "", fmt.Errorf("error serializing chain: %v", err)
	}

	return serializedChain, nil
}

func deserializeChain(serializedChain string) (*consensus.SignedChainTree, error) {
	decodedChain, err := base64.StdEncoding.DecodeString(serializedChain)
	if err != nil {
		return nil, fmt.Errorf("error decoding chain: %v", err)
	}

	chainBuffer := bytes.NewBuffer(decodedChain)
	decoder := gob.NewDecoder(chainBuffer)
	var chain consensus.SignedChainTree

	err = decoder.Decode(&chain)
	if err != nil {
		return nil, fmt.Errorf("error decoding chain: %v", err)
	}

	return &chain, nil
}

func (rpcs *RPCSession) ImportChain(keyAddr string, serializedChain string) (*consensus.SignedChainTree, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	chain, err := deserializeChain(serializedChain)
	if err != nil {
		return nil, err
	}

	rpcs.wallet.SaveChain(chain)

	return chain, nil
}

func (rpcs *RPCSession) GetChainIds() ([]string, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	return rpcs.wallet.GetChainIds()
}

func (rpcs *RPCSession) GetChain(id string) (*consensus.SignedChainTree, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	return rpcs.wallet.GetChain(id)
}

func (rpcs *RPCSession) GetTip(id string) (*cid.Cid, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	tipResp, err := rpcs.client.TipRequest(id)
	if err != nil {
		return nil, err
	}

	if len(tipResp.Signature.NewTip) == 0 {
		return nil, &NilTipError{
			chainId:     id,
			notaryGroup: rpcs.client.Group,
		}
	}

	tip, err := cid.Cast(tipResp.Signature.NewTip)
	if err != nil {
		return nil, err
	}

	return &tip, nil
}

func (rpcs *RPCSession) PlayTransactions(chainId string, keyAddr string, transactions []*chaintree.Transaction) (*consensus.AddBlockResponse, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return nil, err
	}

	key, err := rpcs.getKey(keyAddr)
	if err != nil {
		return nil, err
	}

	var remoteTip string
	if !chain.IsGenesis() {
		remoteTip = chain.Tip().String()
	}

	resp, err := rpcs.client.PlayTransactions(chain, key, remoteTip, transactions)
	if err != nil {
		return nil, err
	}

	err = rpcs.wallet.SaveChain(chain)
	if err != nil {
		return nil, fmt.Errorf("Error saving chain: %v", err)
	}

	return resp, nil
}

func (rpcs *RPCSession) SetOwner(chainId string, keyAddr string, newOwnerKeyAddrs []string) (*cid.Cid, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeSetOwnership,
			Payload: consensus.SetOwnershipPayload{
				Authentication: newOwnerKeyAddrs,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return resp.Tip, nil
}

func (rpcs *RPCSession) SetData(chainId string, keyAddr string, path string, value []byte) (*cid.Cid, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	var deserializedVal interface{}
	err := cbornode.DecodeInto(value, &deserializedVal)
	if err != nil {
		return nil, fmt.Errorf("error decoding value: %v", err)
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeSetData,
			Payload: consensus.SetDataPayload{
				Path:  path,
				Value: deserializedVal,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return resp.Tip, nil
}

func (rpcs *RPCSession) Resolve(chainId string, path []string) (interface{}, []string, error) {
	if rpcs.IsStopped() {
		return nil, nil, StoppedError
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return nil, nil, err
	}

	treePath := append([]string{"tree"}, path...)
	return chain.ChainTree.Dag.Resolve(treePath)
}

func (rpcs *RPCSession) EstablishCoin(chainId string, keyAddr string, coinName string, amount uint64) (*cid.Cid, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeEstablishCoin,
			Payload: consensus.EstablishCoinPayload{
				Name:           coinName,
				MonetaryPolicy: consensus.CoinMonetaryPolicy{Maximum: amount},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return resp.Tip, nil
}

func (rpcs *RPCSession) MintCoin(chainId string, keyAddr string, coinName string, amount uint64) (*cid.Cid, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeMintCoin,
			Payload: consensus.MintCoinPayload{
				Name:   coinName,
				Amount: amount,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return resp.Tip, nil
}
