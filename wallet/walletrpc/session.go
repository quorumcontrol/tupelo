package walletrpc

import (
	"crypto/ecdsa"
	"path/filepath"

	cid "github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossipclient"
	"github.com/quorumcontrol/qc3/wallet"
)

type RPCSession struct {
	client *gossipclient.GossipClient
	wallet *wallet.FileWallet
}

func walletPath(name string) string {
	return filepath.Join(".storage", name+"-wallet")
}

func NewSession(creds *Credentials, group *consensus.Group) *RPCSession {
	path := walletPath(creds.WalletName)
	fileWallet := wallet.NewFileWallet(creds.PassPhrase, path)

	gossipClient := gossipclient.NewGossipClient(group)
	gossipClient.Start()

	return &RPCSession{
		client: gossipClient,
		wallet: fileWallet,
	}
}

func (rpcs *RPCSession) GenerateKey() (*ecdsa.PrivateKey, error) {
	return rpcs.wallet.GenerateKey()
}

func (rpcs *RPCSession) ListKeys() ([]string, error) {
	return rpcs.wallet.ListKeys()
}

func (rpcs *RPCSession) getKey(keyAddr string) (*ecdsa.PrivateKey, error) {
	return rpcs.wallet.GetKey(keyAddr)
}

func (rpcs *RPCSession) CreateChain(keyAddr string) (*consensus.SignedChainTree, error) {
	key, err := rpcs.getKey(keyAddr)
	if err != nil {
		return nil, err
	}

	chain, err := consensus.NewSignedChainTree(key.PublicKey)
	if err != nil {
		return nil, err
	}

	rpcs.wallet.SaveChain(chain)
	return chain, nil
}

func (rpcs *RPCSession) GetChainIds() ([]string, error) {
	return rpcs.wallet.GetChainIds()
}

func (rpcs *RPCSession) GetChain(id string) (*consensus.SignedChainTree, error) {
	return rpcs.wallet.GetChain(id)
}

func (rpcs *RPCSession) GetTip(id string) (*cid.Cid, error) {
	tipResp, err := rpcs.client.TipRequest(id)
	if err != nil {
		return nil, err
	}

	return tipResp.Tip, nil
}

func (rpcs *RPCSession) prepareTransaction(chainId string, keyAddr string) (*consensus.SignedChainTree, *ecdsa.PrivateKey, string, error) {
	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return nil, nil, "", err
	}

	key, err := rpcs.getKey(keyAddr)
	if err != nil {
		return nil, nil, "", err
	}

	var remoteTip string
	if !chain.IsGenesis() {
		remoteTip = chain.Tip().String()
	}

	return chain, key, remoteTip, nil
}

func (rpcs *RPCSession) PlayTransactions(tree *consensus.SignedChainTree, treeKey *ecdsa.PrivateKey, remoteTip string, transactions []*chaintree.Transaction) (*consensus.AddBlockResponse, error) {
	return rpcs.client.PlayTransactions(tree, treeKey, remoteTip, transactions)
}

func (rpcs *RPCSession) SetOwner(chainId string, keyAddr string, newOwnerKeys []*consensus.PublicKey, path string, value string) (*cid.Cid, error) {
	chain, key, remoteTip, err := rpcs.prepareTransaction(chainId, keyAddr)
	if err != nil {
		return nil, err
	}

	resp, err := rpcs.PlayTransactions(chain, key, remoteTip, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeSetOwnership,
			Payload: consensus.SetOwnershipPayload{
				Authentication: newOwnerKeys,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	rpcs.wallet.SaveChain(chain)
	return resp.Tip, nil
}

func (rpcs *RPCSession) SetData(chainId string, keyAddr string, path string, value string) (*cid.Cid, error) {
	chain, key, remoteTip, err := rpcs.prepareTransaction(chainId, keyAddr)
	if err != nil {
		return nil, err
	}

	resp, err := rpcs.PlayTransactions(chain, key, remoteTip, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeSetData,
			Payload: consensus.SetDataPayload{
				Path:  path,
				Value: value,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	rpcs.wallet.SaveChain(chain)
	return resp.Tip, nil
}

// func (rpcs *RPCSession) EstablishCoin(id string, path string, value string) (*consensus.AddBlockResponse, error) {

// }

// func (rpcs *RPCSession) MintCoin(id string, path string, value string) (*consensus.AddBlockResponse, error) {

// }
