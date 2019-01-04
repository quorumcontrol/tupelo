package walletrpc

import (
	"crypto/ecdsa"
	"errors"
	fmt "fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gogo/protobuf/proto"
	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip2client"
	"github.com/quorumcontrol/tupelo/wallet"
)

type RPCSession struct {
	client    *gossip2client.GossipClient
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
	notaryGroup *consensus.NotaryGroup
}

func (e *NilTipError) Error() string {
	return fmt.Sprintf("Chain tree with id %v is not known to the notary group %v", e.chainId, e.notaryGroup)
}

func NewSession(storagePath string, walletName string, gossipClient *gossip2client.GossipClient) (*RPCSession, error) {
	path := walletPath(storagePath, walletName)

	fileWallet := wallet.NewFileWallet(path)

	return &RPCSession{
		client:    gossipClient,
		wallet:    fileWallet,
		isStarted: false,
	}, nil
}

func decodeDag(encodedDag [][]byte, store nodestore.NodeStore) (*dag.Dag, error) {
	dagNodes := make([]*cbornode.Node, len(encodedDag))

	for i, rawNode := range encodedDag {
		block := blocks.NewBlock(rawNode)
		node, err := cbornode.DecodeBlock(block)
		if err != nil {
			return nil, err
		}

		dagNodes[i] = node.(*cbornode.Node)
	}

	return dag.NewDagWithNodes(store, dagNodes...)
}

func serializeDag(dag *dag.Dag) ([][]byte, error) {
	dagNodes, err := dag.Nodes()
	if err != nil {
		return nil, err
	}

	dagBytes := make([][]byte, len(dagNodes))
	for i, node := range dagNodes {
		dagBytes[i] = node.RawData()
	}

	return dagBytes, nil
}

func decodeSignatures(encodedSigs map[string]*SerializableSignature) (consensus.SignatureMap, error) {
	signatures := make(consensus.SignatureMap)

	for k, encodedSig := range encodedSigs {
		signature := consensus.Signature{
			Type:      encodedSig.Type,
			Signers:   encodedSig.Signers,
			Signature: encodedSig.Signature,
		}

		signatures[k] = signature
	}

	return signatures, nil
}

func serializeSignatures(sigs consensus.SignatureMap) map[string]*SerializableSignature {
	serializedSigs := make(map[string]*SerializableSignature)
	for k, sig := range sigs {
		serializedSigs[k] = &SerializableSignature{
			Signers:   sig.Signers,
			Signature: sig.Signature,
			Type:      sig.Type,
		}
	}

	return serializedSigs
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
	tip, _ := rpcs.wallet.GetTip(chainId)

	return tip != nil && len(tip) > 0
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

	chain, err := consensus.NewSignedChainTree(key.PublicKey, rpcs.wallet.NodeStore())
	if err != nil {
		return nil, err
	}

	rpcs.wallet.SaveChain(chain)
	return chain, nil
}

func (rpcs *RPCSession) ExportChain(chainId string) ([]byte, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return nil, err
	}

	dagBytes, err := serializeDag(chain.ChainTree.Dag)
	if err != nil {
		return nil, err
	}

	serializedSigs := serializeSignatures(chain.Signatures)

	serializableChain := SerializableChainTree{
		Dag:        dagBytes,
		Signatures: serializedSigs,
	}

	serializedChain, err := proto.Marshal(&serializableChain)
	if err != nil {
		return nil, err
	}

	return serializedChain, nil
}

func (rpcs *RPCSession) ImportChain(keyAddr string, serializedChain []byte) (*consensus.SignedChainTree, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	decodedChain := &SerializableChainTree{}
	err := proto.Unmarshal(serializedChain, decodedChain)
	if err != nil {
		return nil, err
	}

	dag, err := decodeDag(decodedChain.Dag, rpcs.wallet.NodeStore())
	if err != nil {
		return nil, err
	}

	chainTree, err := chaintree.NewChainTree(dag, nil, consensus.DefaultTransactors)
	if err != nil {
		return nil, err
	}

	sigs, err := decodeSignatures(decodedChain.Signatures)
	if err != nil {
		return nil, err
	}

	signedChainTree := &consensus.SignedChainTree{
		ChainTree:  chainTree,
		Signatures: sigs,
	}

	rpcs.wallet.SaveChain(signedChainTree)

	return signedChainTree, nil
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

	if len(tipResp.Tip) == 0 {
		return nil, &NilTipError{
			chainId:     id,
			notaryGroup: rpcs.client.Group,
		}
	}

	tip, err := cid.Cast(tipResp.Tip)
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
	if rpcs.IsStopped() {
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
