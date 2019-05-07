package walletrpc

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/jakehl/goid"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"

	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/client"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	extmsgs "github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	gossip3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/wallet"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
)

type RPCSession struct {
	pubsub      remote.PubSub
	notaryGroup *types.NotaryGroup
	wallet      *wallet.FileWallet
	isStarted   bool
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

func NewSession(storagePath string, walletName string, notaryGroup *types.NotaryGroup, pubsub remote.PubSub) (*RPCSession, error) {
	path := walletPath(storagePath, walletName)

	fileWallet := wallet.NewFileWallet(path)

	return &RPCSession{
		pubsub:      pubsub,
		notaryGroup: notaryGroup,
		wallet:      fileWallet,
		isStarted:   false,
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

func decodeSignature(encodedSig *SerializableSignature) (*extmsgs.Signature, error) {
	signers := bitarray.NewBitArray(uint64(len(encodedSig.Signers)))
	for i, didSign := range encodedSig.Signers {
		if didSign {
			if err := signers.SetBit(uint64(i)); err != nil {
				return nil, fmt.Errorf("error setting bit: %v", err)
			}
		}
	}
	marshalledSigners, err := bitarray.Marshal(signers)
	if err != nil {
		return nil, fmt.Errorf("error marshalling signers array: %v", err)
	}

	return &extmsgs.Signature{
		ObjectID:    encodedSig.ObjectId,
		PreviousTip: encodedSig.PreviousTip,
		NewTip:      encodedSig.NewTip,
		View:        encodedSig.View,
		Cycle:       encodedSig.Cycle,
		Type:        encodedSig.Type,
		Signers:     marshalledSigners,
		Signature:   encodedSig.Signature,
		Height:      encodedSig.Height,
	}, nil
}

func decodeSignatures(encodedSigs map[string]*SerializableSignature) (consensus.SignatureMap, error) {
	signatures := make(consensus.SignatureMap)

	for k, encodedSig := range encodedSigs {
		decodedSig, err := decodeSignature(encodedSig)
		if err != nil {
			return nil, fmt.Errorf("error decoding signature: %v", err)
		}
		signatures[k] = *decodedSig
	}

	return signatures, nil
}

func (rpcs *RPCSession) verifySignatures(sigs consensus.SignatureMap) (bool, error) {
	var verKeys [][]byte

	sigForNotaryGroup := sigs[rpcs.notaryGroup.ID]
	signerArray, err := bitarray.Unmarshal(sigForNotaryGroup.Signers)
	if err != nil {
		return false, fmt.Errorf("error decoding signatures: %v", err)
	}

	signers := rpcs.notaryGroup.AllSigners()
	for i, signer := range signers {
		isSet, err := signerArray.GetBit(uint64(i))
		if err != nil {
			return false, fmt.Errorf("error converting signatures: %v", err)
		}
		if isSet {
			verKeys = append(verKeys, signer.VerKey.Bytes())
		}
	}

	return bls.VerifyMultiSig(sigForNotaryGroup.Signature, sigForNotaryGroup.GetSignable(), verKeys)
}

func serializeSignature(sig extmsgs.Signature) (*SerializableSignature, error) {
	signers, err := bitarray.Unmarshal(sig.Signers)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling signers array: %v", err)
	}

	signersBools := make([]bool, signers.Capacity())
	for i := uint64(0); i < signers.Capacity(); i++ {
		isSet, err := signers.GetBit(i)
		if err != nil {
			return nil, fmt.Errorf("error getting signer from bitarray: %v", err)
		}

		signersBools[i] = isSet
	}

	return &SerializableSignature{
		ObjectId:    sig.ObjectID,
		PreviousTip: sig.PreviousTip,
		NewTip:      sig.NewTip,
		View:        sig.View,
		Cycle:       sig.Cycle,
		Signers:     signersBools,
		Signature:   sig.Signature,
		Type:        sig.Type,
		Height:      sig.Height,
	}, nil
}

func serializeSignatures(sigs consensus.SignatureMap) (map[string]*SerializableSignature, error) {
	serializedSigs := make(map[string]*SerializableSignature)
	for k, sig := range sigs {
		serialized, err := serializeSignature(sig)
		if err != nil {
			return nil, fmt.Errorf("error serializing signature: %v", err)
		}
		serializedSigs[k] = serialized
	}

	return serializedSigs, nil
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

	serializedSigs, err := serializeSignatures(chain.Signatures)
	if err != nil {
		return "", err
	}

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

func (rpcs *RPCSession) ImportChain(serializedChain string, shouldValidate bool, storageAdapterConfig *adapters.Config) (*consensus.SignedChainTree, error) {
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

	sigs, err := decodeSignatures(unmarshalledChain.Signatures)
	if err != nil {
		return nil, fmt.Errorf("error decoding signatures: %v", err)
	}

	if shouldValidate {
		isValid, err := rpcs.verifySignatures(sigs)
		if err != nil {
			return nil, fmt.Errorf("error verifying signatures: %v", err)
		}
		if !isValid {
			return nil, fmt.Errorf("signatures for ChainTree are invalid for notary group %v", rpcs.notaryGroup.ID)
		}
	}

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

	client := client.New(rpcs.notaryGroup, id, rpcs.pubsub)
	client.Listen()
	defer client.Stop()

	tipResp, err := client.TipRequest()
	if err != nil {
		return nil, err
	}

	if len(tipResp.Signature.NewTip) == 0 {
		return nil, &NilTipError{
			chainId:     id,
			notaryGroup: rpcs.notaryGroup,
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

	cli := client.New(rpcs.notaryGroup, chainId, rpcs.pubsub)
	cli.Listen()
	defer cli.Stop()

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

	resp, err := cli.PlayTransactions(chain, key, &remoteTip, transactions)
	if err != nil {
		return nil, err
	}

	err = rpcs.wallet.SaveChainMetadata(chain)
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
			Type: consensus.TransactionTypeSendToken,
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

	tree, err := chain.ChainTree.Tree()
	if err != nil {
		return "", err
	}

	canonicalTokenName, err := consensus.CanonicalTokenName(tree, chainId, tokenName, false)
	if err != nil {
		return "", err
	}

	tokenSends, err := consensus.TokenTransactionCidsForType(chain.ChainTree.Dag, canonicalTokenName.String(), consensus.TokenSendLabel)
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

	tokenNodes, err := allSendTokenNodes(chain, canonicalTokenName.String(), tokenSendTx)
	if err != nil {
		return "", err
	}

	tip := *resp.Tip

	serialized, err := serializeSignature(resp.Signature)
	if err != nil {
		return "", err
	}

	payload := &TokenPayload{
		TransactionId: transactionId.String(),
		Tip:           tip.String(),
		Signature:     serialized,
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

	decodedSig, err := decodeSignature(tokenPayload.Signature)
	if err != nil {
		return nil, err
	}

	resp, err := rpcs.PlayTransactions(chainId, keyAddr, []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeReceiveToken,
			Payload: consensus.ReceiveTokenPayload{
				SendTokenTransactionId: tokenPayload.TransactionId,
				Tip:                    tip.Bytes(),
				Signature:              *decodedSig,
				Leaves:                 tokenPayload.Leaves,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return resp.Tip, nil
}

// GetTokenBalance gets the balance for a certain token in the tree.
func (rpcs *RPCSession) GetTokenBalance(chainId, token string) (uint64, error) {
	if rpcs.IsStopped() {
		return 0, StoppedError
	}

	chain, err := rpcs.GetChain(chainId)
	if err != nil {
		return 0, err
	}

	tree, err := chain.ChainTree.Tree()
	if err != nil {
		return 0, err
	}
	canonicalTokenName, err := consensus.CanonicalTokenName(tree, chainId, token, false)
	if err != nil {
		return 0, err
	}
	ledger := consensus.NewTreeLedger(tree, canonicalTokenName.String())
	bal, err := ledger.Balance()
	if err != nil {
		return 0, fmt.Errorf("error getting token %s balance: %s", token, err)
	}

	return bal, nil
}

func (rpcs *RPCSession) ListTokens(chainId string) (string, error) {
	tokensPath, err := consensus.DecodePath("tree/_tupelo/tokens")
	if err != nil {
		return "", err
	}

	tokensObj, remaining, err := rpcs.Resolve(chainId, tokensPath)
	if err != nil {
		return "", err
	}
	if len(remaining) > 0 {
		return "", nil // no tokens yet
	}

	tokens, ok := tokensObj.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("tokens node was of unexpected type")
	}

	tokensList := ""

	i := 0
	for name, tokenObj := range tokens {
		tokenCid, ok := tokenObj.(cid.Cid)
		if !ok {
			continue // I guess?
		}

		chain, err := rpcs.GetChain(chainId)
		if err != nil {
			return "", err
		}
		tokenNode, err := chain.ChainTree.Dag.Get(tokenCid)
		if err != nil {
			return "", err
		}

		token := consensus.Token{}
		err = cbornode.DecodeInto(tokenNode.RawData(), &token)
		if err != nil {
			return "", err
		}

		tokensList += fmt.Sprintf("%d: %s (balance: %d)\n", i, name, token.Balance)

		i++
	}

	return tokensList, nil
}
