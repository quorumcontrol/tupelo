package walletrpc

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	fmt "fmt"
	"log"
	"path/filepath"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gogo/protobuf/proto"
	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	gossip3client "github.com/quorumcontrol/tupelo-go-client/client"
	"github.com/quorumcontrol/tupelo-go-client/consensus"
	gossip3types "github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/wallet"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
	"github.com/jakehl/goid"
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

func decodeDag(encodedDag [][]byte, store nodestore.NodeStore) (*dag.Dag, error) {
	dagNodes := make([]*cbornode.Node, len(encodedDag))

	sw := &safewrap.SafeWrap{}
	for i, rawNode := range encodedDag {
		dagNodes[i] = sw.Decode(rawNode)
	}
	if sw.Err != nil {
		return nil, fmt.Errorf("error decoding: %v", sw.Err)
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

func serializeNodes(nodes []*cbornode.Node) [][]byte {
	var bytes [][]byte
	for _, node := range nodes {
		bytes = append(bytes, node.RawData())
	}
	return bytes
}

func decodeSignature(encodedSig *SerializableSignature) consensus.Signature {
	return consensus.Signature{
		Type:      encodedSig.Type,
		Signers:   encodedSig.Signers,
		Signature: encodedSig.Signature,
	}
}

func decodeSignatures(encodedSigs map[string]*SerializableSignature) consensus.SignatureMap {
	signatures := make(consensus.SignatureMap)

	for k, encodedSig := range encodedSigs {
		signatures[k] = decodeSignature(encodedSig)
	}

	return signatures
}

func serializeSignature(sig consensus.Signature) *SerializableSignature {
	return &SerializableSignature{
		Signers:   sig.Signers,
		Signature: sig.Signature,
		Type:      sig.Type,
	}
}

func serializeSignatures(sigs consensus.SignatureMap) map[string]*SerializableSignature {
	serializedSigs := make(map[string]*SerializableSignature)
	for k, sig := range sigs {
		serializedSigs[k] = serializeSignature(sig)
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
	return rpcs.wallet.ChainExists(chainId)
}

func (rpcs *RPCSession) defaultChainStorage(chainId string) *adapters.Config {
	return &adapters.Config{
		Adapter: adapters.BadgerStorageAdapterName,
		Arguments: map[string]interface{}{
			"path": rpcs.wallet.Path() + "-" + chainId,
		},
	}
}

func (rpcs *RPCSession) CreateChain(keyAddr string, storageAdapterConfig *adapters.Config) (*consensus.SignedChainTree, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	key, err := rpcs.getKey(keyAddr)
	if err != nil {
		return nil, fmt.Errorf("error getting key: %v", err)
	}

	if rpcs.chainExists(key.PublicKey) {
		return nil, ExistingChainError{publicKey: &key.PublicKey}
	}

	if storageAdapterConfig == nil {
		chainId := consensus.EcdsaPubkeyToDid(key.PublicKey)
		storageAdapterConfig = rpcs.defaultChainStorage(chainId)
	}

	return rpcs.wallet.CreateChain(keyAddr, storageAdapterConfig)
}

func (rpcs *RPCSession) ExportChain(chainId string) (string, error) {
	if rpcs.IsStopped() {
		return "", StoppedError
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return "", err
	}

	dagBytes, err := serializeDag(chain.ChainTree.Dag)
	if err != nil {
		return "", err
	}

	serializedSigs := serializeSignatures(chain.Signatures)

	serializableChain := SerializableChainTree{
		Dag:        dagBytes,
		Signatures: serializedSigs,
		Tip:        chain.ChainTree.Dag.Tip.String(),
	}

	serializedChain, err := proto.Marshal(&serializableChain)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(serializedChain), nil
}

func (rpcs *RPCSession) ImportChain(serializedChain string, storageAdapterConfig *adapters.Config) (*consensus.SignedChainTree, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	decodedChain, err := base64.StdEncoding.DecodeString(serializedChain)
	if err != nil {
		return nil, fmt.Errorf("error decoding chain: %v", err)
	}

	unmarshalledChain := &SerializableChainTree{}
	err = proto.Unmarshal(decodedChain, unmarshalledChain)
	if err != nil {
		return nil, err
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	chainDAG, err := decodeDag(unmarshalledChain.Dag, nodeStore)
	if err != nil {
		return nil, fmt.Errorf("error decoding dag: %v", err)
	}

	tip, err := cid.Decode(unmarshalledChain.Tip)
	if err != nil {
		return nil, fmt.Errorf("error decoding tip CID: %v", err)
	}

	chainDAG = chainDAG.WithNewTip(tip)

	chainTree, err := chaintree.NewChainTree(chainDAG, nil, consensus.DefaultTransactors)
	if err != nil {
		return nil, fmt.Errorf("error getting new chaintree: %v", err)
	}

	sigs := decodeSignatures(unmarshalledChain.Signatures)

	signedChainTree := &consensus.SignedChainTree{
		ChainTree:  chainTree,
		Signatures: sigs,
	}

	if storageAdapterConfig == nil {
		storageAdapterConfig = rpcs.defaultChainStorage(signedChainTree.MustId())
	}

	if err = rpcs.wallet.ConfigureChainStorage(signedChainTree.MustId(), storageAdapterConfig); err != nil {
		log.Printf("failed to configure chain storage (chain ID: %s): %s",
			signedChainTree.MustId(), err)
		// TODO: Enable
		// return nil, fmt.Errorf("failed to configure chain storage (chain ID: %s): %s",
		// 	signedChainTree.MustId(), err)
	}
	if err = rpcs.wallet.SaveChain(signedChainTree); err != nil {
		log.Printf("failed to save chain tree with ID %s: %s", signedChainTree.MustId(), err)
		// TODO: Enable
		// return nil, fmt.Errorf("failed to save chain tree with ID %s: %s",
		// signedChainTree.MustId(), err)
	}

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

func (rpcs *RPCSession) PlayTransactions(chainId, keyAddr string, transactions []*chaintree.Transaction) (*consensus.AddBlockResponse, error) {
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

	var remoteTip cid.Cid
	if !chain.IsGenesis() {
		remoteTip = chain.Tip()
	}

	resp, err := rpcs.client.PlayTransactions(chain, key, &remoteTip, transactions)
	if err != nil {
		return nil, err
	}

	err = rpcs.wallet.SaveChain(chain)
	if err != nil {
		return nil, fmt.Errorf("error saving chain: %v", err)
	}

	return resp, nil
}

func (rpcs *RPCSession) SetOwner(chainId, keyAddr string, newOwnerKeyAddrs []string) (*cid.Cid, error) {
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

func (rpcs *RPCSession) SetData(chainId, keyAddr, path string, value []byte) (*cid.Cid, error) {
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
	return rpcs.resolveAt(chainId, path, nil)
}

func (rpcs *RPCSession) resolveAt(chainId string, path []string, tip *cid.Cid) (interface{},
	[]string, error) {
	if rpcs.IsStopped() {
		return nil, nil, StoppedError
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return nil, nil, err
	}

	if tip == nil {
		tip = &chain.ChainTree.Dag.Tip
	}

	return chain.ChainTree.Dag.ResolveAt(*tip, path)
}

func (rpcs *RPCSession) EstablishToken(chainId, keyAddr, tokenName string, amount uint64) (*cid.Cid, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeEstablishToken,
			Payload: consensus.EstablishTokenPayload{
				Name:           tokenName,
				MonetaryPolicy: consensus.TokenMonetaryPolicy{Maximum: amount},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return resp.Tip, nil
}

func (rpcs *RPCSession) MintToken(chainId, keyAddr, tokenName string, amount uint64) (*cid.Cid, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeMintToken,
			Payload: consensus.MintTokenPayload{
				Name:   tokenName,
				Amount: amount,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return resp.Tip, nil
}

func allSendTokenNodes(chain *consensus.SignedChainTree, tokenName string, sendNodeId cid.Cid) ([]*cbornode.Node, error) {
	sendTokenNode, codedErr := chain.ChainTree.Dag.Get(sendNodeId)
	if codedErr != nil {
		return nil, fmt.Errorf("error getting send token node: %v", codedErr)
	}

	tokenPath, err := consensus.TokenPath(tokenName)
	if err != nil {
		return nil, err
	}
	tokenPath = append([]string{chaintree.TreeLabel}, tokenPath...)
	tokenPath = append(tokenPath, consensus.TokenSendLabel)

	tokenSendNodes, codedErr := chain.ChainTree.Dag.NodesForPath(tokenPath)
	if codedErr != nil {
		return nil, codedErr
	}

	tokenSendNodes = append(tokenSendNodes, sendTokenNode)

	return tokenSendNodes, nil
}

func (rpcs *RPCSession) SendToken(chainId, keyAddr, tokenName, destinationChainId string, amount uint64) (string, error) {
	if rpcs.IsStopped() {
		return "", StoppedError
	}

	transactionId := goid.NewV4UUID()

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type:    consensus.TransactionTypeSendToken,
			Payload: consensus.SendTokenPayload{
				Id:          transactionId.String(),
				Name:        tokenName,
				Amount:      amount,
				Destination: destinationChainId,
			},
		},
	})
	if err != nil {
		return "", err
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return "", err
	}

	tokenSends, err := consensus.TokenTransactionCidsForType(chain.ChainTree.Dag, tokenName, consensus.TokenSendLabel)
	if err != nil {
		return "", err
	}

	tokenSendTx := cid.Undef
	for _, sendTxCid := range tokenSends {
		sendTxNode, err := chain.ChainTree.Dag.Get(sendTxCid)
		if err != nil {
			return "", err
		}

		sendTxNodeObj, err := nodestore.CborNodeToObj(sendTxNode)
		if err != nil {
			return "", err
		}

		sendTxNodeMap := sendTxNodeObj.(map[string]interface{})
		if sendTxNodeMap["id"] == transactionId.String() {
			tokenSendTx = sendTxCid
			break
		}
	}

	if !tokenSendTx.Defined() {
		return "", fmt.Errorf("send token transaction not found for ID: %s", transactionId.String())
	}

	tokenNodes, err := allSendTokenNodes(chain, tokenName, tokenSendTx)
	if err != nil {
		return "", err
	}

	tip := *resp.Tip

	payload := &TokenPayload{
		TransactionId: transactionId.String(),
		Tip:           tip.String(),
		Signature:     serializeSignature(resp.Signature),
		Leaves:        serializeNodes(tokenNodes),
	}

	serializedPayload, err := proto.Marshal(payload)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(serializedPayload), nil
}

func (rpcs *RPCSession) ReceiveToken(chainId, keyAddr, payload string) (*cid.Cid, error) {
	if rpcs.IsStopped() {
		return nil, StoppedError
	}

	serializedPayload, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, err
	}

	tokenPayload := &TokenPayload{}
	err = proto.Unmarshal(serializedPayload, tokenPayload)
	if err != nil {
		return nil, err
	}

	tip, err := cid.Decode(tokenPayload.Tip)
	if err != nil {
		return nil, err
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type:    consensus.TransactionTypeReceiveToken,
			Payload: consensus.ReceiveTokenPayload{
				SendTokenTransactionId: tokenPayload.TransactionId,
				Tip:                    tip.Bytes(),
				Signature:              decodeSignature(tokenPayload.Signature),
				Leaves:                 tokenPayload.Leaves,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return resp.Tip, nil
}
