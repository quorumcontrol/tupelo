// +build wasm

package jsclient

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"syscall/js"
	"time"

	"github.com/quorumcontrol/tupelo/sdk/bls"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/tupelo/sdk/gossip/client"
	"github.com/quorumcontrol/tupelo/sdk/gossip/client/pubsubinterfaces"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	prooftype "github.com/quorumcontrol/tupelo/sdk/proof"

	"github.com/quorumcontrol/messages/v2/build/go/config"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"

	"github.com/quorumcontrol/tupelo/sdk/wasm/jsstore"

	"github.com/gogo/protobuf/proto"

	"github.com/pkg/errors"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/quorumcontrol/tupelo/sdk/consensus"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/sdk/wasm/helpers"
	"github.com/quorumcontrol/tupelo/sdk/wasm/then"
)

// JSClient is a javascript bridging client
type JSClient struct {
	client      *client.Client
	pubsub      pubsubinterfaces.Pubsubber
	notaryGroup *types.NotaryGroup
	store       nodestore.DagStore
}

func New(pubsub pubsubinterfaces.Pubsubber, humanConfig *config.NotaryGroup, store nodestore.DagStore) *JSClient {
	ngConfig, err := types.HumanConfigToConfig(humanConfig)
	if err != nil {
		panic(errors.Wrap(err, "error decoding human config"))
	}

	ng, err := ngConfig.NotaryGroup(nil)
	if err != nil {
		panic(errors.Wrap(err, "error getting notary group from config"))
	}

	cli := client.New(WithNotaryGroup(ng), WithPubsub(pubsub), WithDagStore(store))

	return &JSClient{
		client:      cli,
		pubsub:      pubsub,
		notaryGroup: ng,
		store:       store,
	}
}

func (jsc *JSClient) Start(ctx context.Context) error {
	return jsc.client.Start(ctx)
}

func jsTransactionsToTransactions(jsTransactions js.Value) ([]*transactions.Transaction, error) {
	transLength := jsTransactions.Length()
	transBits := make([][]byte, transLength)
	for i := 0; i < transLength; i++ {
		jsVal := jsTransactions.Index(i)
		transBits[i] = helpers.JsBufferToBytes(jsVal)
	}

	trans := make([]*transactions.Transaction, len(transBits))
	for i, bits := range transBits {
		tran := &transactions.Transaction{}
		err := proto.Unmarshal(bits, tran)
		if err != nil {
			return nil, errors.Wrap(err, "error unmarshaling")
		}
		trans[i] = tran
	}
	return trans, nil
}

func jsKeyBitsToPrivateKey(jsKeyBits js.Value) (*ecdsa.PrivateKey, error) {
	keybits := helpers.JsBufferToBytes(jsKeyBits)
	return crypto.ToECDSA(keybits)
}

func (jsc *JSClient) GetTip(jsDid js.Value) interface{} {
	did := jsDid.String()
	t := then.New()
	go func() {
		ctx := context.TODO()
		proof, err := jsc.client.GetTip(ctx, did)
		if err != nil {
			t.Reject(fmt.Errorf("error getting tip: %w", err))
			return
		}

		bits, err := proof.Marshal()
		if err != nil {
			t.Reject(err)
			return
		}

		t.Resolve(helpers.SliceToJSArray(bits))
	}()
	return t
}

// in the js client both GetLatest and GetTip return a proof
// but this one will also play the transactions... see client.Client#GetLatest
// in order to make the new blocks available
func (jsc *JSClient) GetLatest(jsDid js.Value) interface{} {
	did := jsDid.String()
	t := then.New()
	go func() {
		ctx := context.TODO()
		tree, err := jsc.client.GetLatest(ctx, did)
		if err != nil {
			t.Reject(fmt.Errorf("error getting tip: %w", err))
			return
		}

		proof := tree.Proof

		bits, err := proof.Marshal()
		if err != nil {
			t.Reject(err)
			return
		}

		t.Resolve(helpers.SliceToJSArray(bits))
	}()
	return t
}

func (jsc *JSClient) PlayTransactions(jsKeyBits js.Value, tip js.Value, jsTransactions js.Value) interface{} {
	t := then.New()
	go func() {
		trans, err := jsTransactionsToTransactions(jsTransactions)
		if err != nil {
			t.Reject(err)
			return
		}

		key, err := jsKeyBitsToPrivateKey(jsKeyBits)
		if err != nil {
			t.Reject(err)
			return
		}

		tip, err := helpers.JsCidToCid(tip)
		if err != nil {
			t.Reject(err)
			return
		}

		proof, err := jsc.playTransactions(key, tip, trans)
		if err != nil {
			t.Reject(err)
			return
		}

		bits, err := proof.Marshal()
		if err != nil {
			t.Reject(err)
			return
		}

		t.Resolve(helpers.SliceToJSArray(bits))
	}()

	return t
}

func (jsc *JSClient) WaitForRound() *then.Then {
	t := then.New()
	go func() {
		err := jsc.client.WaitForFirstRound(context.TODO(), 30*time.Second)
		if err != nil {
			t.Reject(err)
			return
		}

		t.Resolve(nil)
	}()

	return t
}

func (jsc *JSClient) playTransactions(treeKey *ecdsa.PrivateKey, tip cid.Cid, transactions []*transactions.Transaction) (*gossip.Proof, error) {
	ctx := context.TODO()

	cTree, err := chaintree.NewChainTree(
		ctx,
		dag.NewDag(ctx, tip, jsc.store),
		nil,
		consensus.DefaultTransactors,
	)
	if err != nil {
		return nil, errors.Wrap(err, "error creating chaintree")
	}

	tree := consensus.NewSignedChainTreeFromChainTree(cTree)

	return jsc.client.PlayTransactions(ctx, tree, treeKey, transactions)
}

func (jsc *JSClient) VerifyProof(proofBits js.Value) interface{} {
	t := then.New()
	go func() {
		proof := &gossip.Proof{}
		err := proof.Unmarshal(helpers.JsBufferToBytes(proofBits))
		if err != nil {
			t.Reject(fmt.Errorf("error unmarshaling: %w", err))
			return
		}
		isVerified, err := jsc.verifyProof(proof)
		if err != nil {
			t.Reject(fmt.Errorf("error verifying: %w", err))
			return
		}
		t.Resolve(isVerified)
	}()
	return t
}

func (jsc *JSClient) verifyProof(proof *gossip.Proof) (bool, error) {
	quorumCount := jsc.notaryGroup.QuorumCount()
	signers := jsc.notaryGroup.AllSigners()
	verKeys := make([]*bls.VerKey, len(signers))
	for i, signer := range signers {
		verKeys[i] = signer.VerKey
	}

	return prooftype.Verify(context.TODO(), proof, quorumCount, verKeys)
}

func GenerateKey() *then.Then {
	t := then.New()
	go func() {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Reject(err)
			return
		}
		privateBits := helpers.SliceToJSArray(crypto.FromECDSA(key))
		publicBits := helpers.SliceToJSArray(crypto.FromECDSAPub(&key.PublicKey))
		jsArray := js.Global().Get("Array").New(privateBits, publicBits)
		t.Resolve(jsArray)
	}()
	return t
}

func KeyFromPrivateBytes(jsBits js.Value) *then.Then {
	t := then.New()
	go func() {
		key, err := jsKeyBitsToPrivateKey(jsBits)
		if err != nil {
			t.Reject(err)
			return
		}
		privateBits := helpers.SliceToJSArray(crypto.FromECDSA(key))
		publicBits := helpers.SliceToJSArray(crypto.FromECDSAPub(&key.PublicKey))
		jsArray := js.Global().Get("Array").New(privateBits, publicBits)
		t.Resolve(jsArray)
	}()
	return t
}

func PassPhraseKey(jsPhrase, jsSalt js.Value) *then.Then {
	t := then.New()
	go func() {
		phrase := helpers.JsBufferToBytes(jsPhrase)
		salt := helpers.JsBufferToBytes(jsSalt)
		key, err := consensus.PassPhraseKey(phrase, salt)
		if err != nil {
			t.Reject(err)
			return
		}
		privateBits := helpers.SliceToJSArray(crypto.FromECDSA(key))
		publicBits := helpers.SliceToJSArray(crypto.FromECDSAPub(&key.PublicKey))
		jsArray := js.Global().Get("Array").New(privateBits, publicBits)
		t.Resolve(jsArray)
	}()
	return t
}

func jsToPubKey(jsPubKeyBits js.Value) (*ecdsa.PublicKey, error) {
	pubbits := helpers.JsBufferToBytes(jsPubKeyBits)
	pubKey, err := crypto.UnmarshalPubkey(pubbits)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling public key")
	}
	return pubKey, nil
}

func EcdsaPubkeyToDid(jsPubKeyBits js.Value) *then.Then {
	t := then.New()
	go func() {
		pubKey, err := jsToPubKey(jsPubKeyBits)
		if err != nil {
			t.Reject(err)
			return
		}
		t.Resolve(consensus.EcdsaPubkeyToDid(*pubKey))
	}()
	return t
}

func EcdsaPubkeyToAddress(jsPubKeyBits js.Value) *then.Then {
	t := then.New()
	go func() {
		pubKey, err := jsToPubKey(jsPubKeyBits)
		if err != nil {
			t.Reject(err)
			return
		}
		t.Resolve(crypto.PubkeyToAddress(*pubKey).String())
	}()
	return t
}

// NewEmptyTree is a little departurue from the normal Go SDK, it's a helper for JS to create
// a new blank ChainTree given a private key. It will populate the node store and return the tip
// of the new Dag (so javascript can reconstitute the chaintree on its side.)
func NewEmptyTree(jsBlockService js.Value, jsPublicKeyBits js.Value) *then.Then {
	ctx := context.TODO()

	t := then.New()
	go func() {
		store := jsstore.New(jsBlockService)
		treeKey, err := crypto.UnmarshalPubkey(helpers.JsBufferToBytes(jsPublicKeyBits))
		if err != nil {
			t.Reject(err)
			return
		}
		did := consensus.EcdsaPubkeyToDid(*treeKey)
		dag := consensus.NewEmptyTree(ctx, did, store)
		t.Resolve(helpers.CidToJSCID(dag.Tip))
	}()
	return t
}

func JsConfigToHumanConfig(jsBits js.Value) (*config.NotaryGroup, error) {
	bits := helpers.JsBufferToBytes(jsBits)
	config := &config.NotaryGroup{}
	err := proto.Unmarshal(bits, config)
	return config, err
}

// func TokenPayloadForTransaction(chain *chaintree.ChainTree, tokenName *TokenName, sendTokenTxId string, sendTxState *signatures.TreeState) (*transactions.TokenPayload, error) {
func TokenPayloadForTransaction(jsBlockService js.Value, jsTip js.Value, tokenName js.Value, sendTokenTxId js.Value, jsSendTxProofBits js.Value) *then.Then {
	t := then.New()
	ctx := context.TODO()
	go func() {
		wrappedStore := jsstore.New(jsBlockService)
		tip, err := helpers.JsCidToCid(jsTip)
		if err != nil {
			t.Reject(err)
			return
		}

		tree, err := chaintree.NewChainTree(
			ctx,
			dag.NewDag(ctx, tip, wrappedStore),
			nil,
			consensus.DefaultTransactors,
		)
		if err != nil {
			t.Reject(err)
			return
		}

		proofBits := helpers.JsBufferToBytes(jsSendTxProofBits)
		proof := &gossip.Proof{}
		err = proof.Unmarshal(proofBits)
		if err != nil {
			t.Reject(err)
			return
		}

		canonicalTokenName := consensus.TokenNameFromString(tokenName.String())

		payload, err := consensus.TokenPayloadForTransaction(tree, &canonicalTokenName, sendTokenTxId.String(), proof)
		if err != nil {
			t.Reject(err)
			return
		}

		bits, err := proto.Marshal(payload)
		if err != nil {
			t.Reject(err)
			return
		}
		t.Resolve(helpers.SliceToJSArray(bits))
	}()

	return t
}
