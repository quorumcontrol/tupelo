package walletrpc

import (
	"crypto/ecdsa"
	"errors"
	"path/filepath"

	cid "github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossipclient"
	"github.com/quorumcontrol/qc3/wallet"
)

type RPCSession struct {
	client    *gossipclient.GossipClient
	wallet    *wallet.FileWallet
	isStopped bool
}

func walletPath(name string) string {
	return filepath.Join(".storage", name+"-wallet")
}

var StoppedError = errors.New("Session is stopped")

func NewSession(creds *Credentials, group *consensus.NotaryGroup) *RPCSession {
	path := walletPath(creds.WalletName)
	fileWallet := wallet.NewFileWallet(creds.PassPhrase, path)

	gossipClient := gossipclient.NewGossipClient(group)
	gossipClient.Start()

	return &RPCSession{
		client:    gossipClient,
		wallet:    fileWallet,
		isStopped: false,
	}
}

func (rpcs *RPCSession) Stop() {
	rpcs.wallet.Close()
	rpcs.client.Stop()
	rpcs.isStopped = true
}

func (rpcs *RPCSession) GenerateKey() (*ecdsa.PrivateKey, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	return rpcs.wallet.GenerateKey()
}

func (rpcs *RPCSession) ListKeys() ([]string, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	return rpcs.wallet.ListKeys()
}

func (rpcs *RPCSession) getKey(keyAddr string) (*ecdsa.PrivateKey, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	return rpcs.wallet.GetKey(keyAddr)
}

func (rpcs *RPCSession) CreateChain(keyAddr string) (*consensus.SignedChainTree, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	key, err := rpcs.getKey(keyAddr)
	if err != nil {
		return nil, err
	}

	chain, err := consensus.NewSignedChainTree(key.PublicKey, rpcs.wallet.NodeStore())
	if err != nil {
		return nil, err
	}

	rpcs.wallet.SaveChain(chain)
	return chain, nil
}

func (rpcs *RPCSession) GetChainIds() ([]string, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	return rpcs.wallet.GetChainIds()
}

func (rpcs *RPCSession) GetChain(id string) (*consensus.SignedChainTree, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	return rpcs.wallet.GetChain(id)
}

func (rpcs *RPCSession) GetTip(id string) (*cid.Cid, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	tipResp, err := rpcs.client.TipRequest(id)
	if err != nil {
		return nil, err
	}

	return tipResp.Tip, nil
}

func (rpcs *RPCSession) PlayTransactions(chainId string, keyAddr string, transactions []*chaintree.Transaction) (*consensus.AddBlockResponse, error) {
	if rpcs.isStopped {
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

	rpcs.wallet.SaveChain(chain)
	return resp, nil
}

func (rpcs *RPCSession) SetOwner(chainId string, keyAddr string, newOwnerKeyAddrs []string) (*cid.Cid, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	newOwnerKeys := make([]*consensus.PublicKey, len(newOwnerKeyAddrs))
	for i, addr := range newOwnerKeyAddrs {
		k, err := rpcs.getKey(addr)
		if err != nil {
			return nil, err
		}
		pubKey := consensus.EcdsaToPublicKey(&k.PublicKey)
		newOwnerKeys[i] = &pubKey
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
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

	return resp.Tip, nil
}

func (rpcs *RPCSession) SetData(chainId string, keyAddr string, path string, value string) (*cid.Cid, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
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

	return resp.Tip, nil
}
func (rpcs *RPCSession) Resolve(chainId string, path []string) (interface{}, []string, error) {
	if rpcs.isStopped {
		return nil, nil, StoppedError
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return nil, nil, err
	}

	return chain.ChainTree.Dag.Resolve(path)
}

func (rpcs *RPCSession) EstablishCoin(chainId string, keyAddr string, coinName string, amount uint64) (*cid.Cid, error) {
	if rpcs.isStopped {
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
	if rpcs.isStopped {
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
