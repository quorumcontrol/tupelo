// +build wasm

package main

import (
	"context"
	"fmt"
	"syscall/js"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/graftabledag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/sdk/wasm/helpers"
)

// type DagGetter interface {
// 	GetTip(ctx context.Context, did string) (*cid.Cid, error)
// 	GetLatest(ctx context.Context, did string) (*chaintree.ChainTree, error)
// }
// assert fulfills the interface at compile time
var _ graftabledag.DagGetter = (*JsDagGetter)(nil)

type JsDagGetter struct {
	tipGetter    js.Value
	store        nodestore.DagStore
	group        *types.NotaryGroup
	validators   []chaintree.BlockValidatorFunc
	transactions map[transactions.Transaction_Type]chaintree.TransactorFunc
}

func NewJsDagGetter(ctx context.Context, group *types.NotaryGroup, tipGetter js.Value, store nodestore.DagStore) (*JsDagGetter, error) {
	validators, err := group.BlockValidators(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting validators: %v", err)
	}
	return &JsDagGetter{
		tipGetter:    tipGetter,
		store:        store,
		group:        group,
		validators:   validators,
		transactions: group.Config().Transactions,
	}, nil
}

func (jdg *JsDagGetter) GetTip(ctx context.Context, did string) (*cid.Cid, error) {
	resp := make(chan js.Value, 1)
	errorResp := make(chan error, 1)

	onError := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		errorResp <- fmt.Errorf("error getting tip: %s", args[0].String())
		return nil
	})

	onSuccess := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		resp <- args[0]
		return nil
	})

	defer func() {
		onSuccess.Release()
		onError.Release()
		close(resp)
		close(errorResp)
	}()

	go func() {
		promise := jdg.tipGetter.Invoke(js.ValueOf(did))
		promise.Call("then", onSuccess, onError)
	}()

	select {
	case err := <-errorResp:
		return nil, err
	case jsTip := <-resp:
		if jsTip.IsUndefined() {
			return nil, chaintree.ErrTipNotFound
		}
		// otherwise we have the tip bytes
		tip, err := helpers.JsCidToCid(jsTip)
		if err != nil {
			return nil, fmt.Errorf("error getting JS CID: %w", err)
		}
		return &tip, nil
	}
}

func (jdg *JsDagGetter) GetLatest(ctx context.Context, did string) (*chaintree.ChainTree, error) {
	tip, err := jdg.GetTip(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("error getting tip: %w", err)
	}
	treeDag := dag.NewDag(ctx, *tip, jdg.store)

	return chaintree.NewChainTree(ctx, treeDag, jdg.validators, jdg.group.Config().Transactions)
}
