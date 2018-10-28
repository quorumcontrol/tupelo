package walletrpc

import (
	"crypto/ecdsa"
	"errors"
	fmt "fmt"
	"path/filepath"

	"github.com/ipfs/go-ipld-cbor"

	"github.com/btcsuite/btcutil/base58"
	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
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

type NilTipError struct {
	chainId     string
	notaryGroup *consensus.NotaryGroup
}

func (e *NilTipError) Error() string {
	return fmt.Sprintf("Chain tree with id %v is not known to the notary group %v", e.chainId, e.notaryGroup)
}

func RegisterWallet(creds *Credentials) error {
	path := walletPath(creds.WalletName)

	fileWallet := wallet.NewFileWallet(path)
	defer fileWallet.Close()

	return fileWallet.Create(creds.PassPhrase)
}

func NewSession(creds *Credentials, group *consensus.NotaryGroup) (*RPCSession, error) {
	path := walletPath(creds.WalletName)

	fileWallet := wallet.NewFileWallet(path)
	err := fileWallet.Unlock(creds.PassPhrase)
	if err != nil {
		return nil, err
	}

	gossipClient := gossipclient.NewGossipClient(group)
	gossipClient.Start()

	return &RPCSession{
		client:    gossipClient,
		wallet:    fileWallet,
		isStopped: false,
	}, nil
}

func decodeDag(encodedDag []string, store nodestore.NodeStore) (*dag.Dag, error) {
	dagNodes := make([]*cbornode.Node, len(encodedDag))

	for i, nodeString := range encodedDag {
		raw := base58.Decode(nodeString)

		block := blocks.NewBlock(raw)
		node, err := cbornode.DecodeBlock(block)
		if err != nil {
			return nil, err
		}

		dagNodes[i] = node.(*cbornode.Node)
	}

	return dag.NewDagWithNodes(store, dagNodes...)
}

func encodeDag(dag *dag.Dag) ([]string, error) {
	dagNodes, err := dag.Nodes()
	if err != nil {
		return nil, err
	}

	encodedDag := make([]string, len(dagNodes))
	for i, node := range dagNodes {
		raw := node.RawData()
		encodedDag[i] = base58.Encode(raw)
	}

	return encodedDag, nil
}

func decodeSignatures(encodedSigs map[string]*SerializedSignature) (consensus.SignatureMap, error) {
	signatures := make(consensus.SignatureMap)

	for k, encodedSig := range encodedSigs {
		sigBytes := base58.Decode(encodedSig.Signature)

		signature := consensus.Signature{
			Type:      encodedSig.Type,
			Signers:   encodedSig.Signers,
			Signature: sigBytes,
		}

		signatures[k] = signature
	}

	return signatures, nil
}

func serializeSignatures(sigs consensus.SignatureMap) map[string]*SerializedSignature {
	serializedSigs := make(map[string]*SerializedSignature)
	for k, sig := range sigs {
		serializedSigs[k] = &SerializedSignature{
			Signers:   sig.Signers,
			Signature: base58.Encode(sig.Signature),
			Type:      sig.Type,
		}
	}

	return serializedSigs
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

func (rpcs *RPCSession) ExportChain(chainId string) (*SerializedChainTree, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return nil, err
	}

	encodedDag, err := encodeDag(chain.ChainTree.Dag)
	if err != nil {
		return nil, err
	}

	serializedSigs := serializeSignatures(chain.Signatures)

	return &SerializedChainTree{
		Dag:        encodedDag,
		Signatures: serializedSigs,
	}, nil

}

func (rpcs *RPCSession) ImportChain(keyAddr string, serializedChain *SerializedChainTree) (*consensus.SignedChainTree, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	dag, err := decodeDag(serializedChain.Dag, rpcs.wallet.NodeStore())
	if err != nil {
		return nil, err
	}

	chainTree, err := chaintree.NewChainTree(dag, nil, consensus.DefaultTransactors)
	if err != nil {
		return nil, err
	}

	sigs, err := decodeSignatures(serializedChain.Signatures)
	if err != nil {
		return nil, err
	}

	return &consensus.SignedChainTree{
		ChainTree:  chainTree,
		Signatures: sigs,
	}, nil
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

	tip := tipResp.Tip
	if tip == nil {
		return nil, &NilTipError{
			chainId:     id,
			notaryGroup: rpcs.client.Group,
		}
	}

	return tip, nil
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

func (rpcs *RPCSession) SetData(chainId string, keyAddr string, path string, value []byte) (*cid.Cid, error) {
	if rpcs.isStopped {
		return nil, StoppedError
	}

	var decodedVal interface{}
	err := cbornode.DecodeInto(value, &decodedVal)
	if err != nil {
		return nil, fmt.Errorf("error decoding value: %v", err)
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeSetData,
			Payload: consensus.SetDataPayload{
				Path:  path,
				Value: decodedVal,
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

	treePath := append([]string{"tree"}, path...)
	return chain.ChainTree.Dag.Resolve(treePath)
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
