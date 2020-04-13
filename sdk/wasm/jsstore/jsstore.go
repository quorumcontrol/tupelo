// +build wasm

package jsstore

import (
	"context"
	"fmt"
	"syscall/js"

	"github.com/quorumcontrol/tupelo/sdk/wasm/jslibs"

	"github.com/pkg/errors"

	"github.com/quorumcontrol/chaintree/safewrap"

	"github.com/quorumcontrol/tupelo/sdk/wasm/helpers"

	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"

	"github.com/quorumcontrol/chaintree/nodestore"
)

// JSStore is a go wrapper to use a javascript nodestore from wasm
// it implements the nodestore.DagStore interface after being passed an underlying
// javascript block-service implementation (for instance: https://github.com/ipfs/js-ipfs-block-service )
type JSStore struct {
	nodestore.DagStore
	bridged js.Value
}

// New expects a javascript ipfs-block-service (javascript flavor)
// and wraps it to make it compatible with the nodestore.DagStore interface (which is currently just format.DagService)
func New(bridged js.Value) nodestore.DagStore {
	return &JSStore{
		bridged: bridged,
	}
}

func (jss *JSStore) Remove(ctx context.Context, c cid.Cid) error {
	respCh := make(chan error)
	onSuccess := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		respCh <- nil
		return nil
	})
	onError := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		err := fmt.Errorf("error from js: %s", args[0].String())
		respCh <- err
		return nil
	})

	defer func() {
		onSuccess.Release()
		onError.Release()
		close(respCh)
	}()

	go func() {
		promise := jss.bridged.Call("delete", helpers.CidToJSCID(c))
		promise.Call("then", onSuccess, onError)
	}()

	select {
	case resp := <-respCh:
		return resp
	case <-ctx.Done():
		return fmt.Errorf("context done")
	}
	return fmt.Errorf("should never get here")
}

func (jss *JSStore) RemoveMany(ctx context.Context, cids []cid.Cid) error {
	for _, n := range cids {
		err := jss.Remove(ctx, n)
		if err != nil {
			return errors.Wrap(err, "error removing")
		}
	}
	return nil
}

func (jss *JSStore) AddMany(ctx context.Context, nodes []format.Node) error {
	// for now do the slow thing and just loop over, javascript has a putMany[sic] as well, but more annoying to serialize
	for _, n := range nodes {
		err := jss.Add(ctx, n)
		if err != nil {
			return errors.Wrap(err, "error adding")
		}
	}
	return nil
}

func (jss *JSStore) GetMany(ctx context.Context, cids []cid.Cid) <-chan *format.NodeOption {
	// javascript has a getMany method, but it's complicated to use.
	panic("don't CALL ME, I'm unimplemented and full of choclate")
}

func (jss *JSStore) Add(ctx context.Context, n format.Node) error {
	respCh := make(chan error)

	onSuccess := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		respCh <- nil
		return nil
	})
	onError := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		err := fmt.Errorf("error from js: %s", args[0].String())
		respCh <- err
		return nil
	})

	defer func() {
		onSuccess.Release()
		onError.Release()
		close(respCh)
	}()

	go func() {
		promise := jss.bridged.Call("put", nodeToJSBlock(n))
		promise.Call("then", onSuccess, onError)
	}()
	select {
	case resp := <-respCh:
		return resp
	case <-ctx.Done():
		return fmt.Errorf("context done")
	}
	return fmt.Errorf("should never get here")
}

func (jss *JSStore) Get(ctx context.Context, c cid.Cid) (format.Node, error) {
	respCh := make(chan interface{})
	sw := safewrap.SafeWrap{}

	onSuccess := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		jsBlock := args[0]
		bits := helpers.JsBufferToBytes(jsBlock.Get("data"))
		var n format.Node

		switch c.Type() {
		case cid.DagProtobuf:
			protonode, err := merkledag.DecodeProtobuf(bits)
			if err != nil {
				respCh <- errors.Wrap(err, "error decoding")
				return nil
			}
			n = protonode
		default:
			n = sw.Decode(bits)
		}

		if sw.Err != nil {
			respCh <- errors.Wrap(sw.Err, "error decoding")
			return nil
		}
		respCh <- n
		return nil
	})
	onError := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		err := fmt.Errorf("error from js: %s", args[0].String())
		respCh <- err
		return nil
	})

	defer func() {
		onSuccess.Release()
		onError.Release()
		close(respCh)
	}()

	go func() {
		promise := jss.bridged.Call("get", helpers.CidToJSCID(c))
		promise.Call("then", onSuccess, onError)
	}()

	select {
	case resp := <-respCh:
		switch msg := resp.(type) {
		case error:
			return nil, msg
		case format.Node:
			return msg, nil
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("context done")
	}
	return nil, fmt.Errorf("should never get here")
}

func nodeToJSBlock(n format.Node) js.Value {
	data := helpers.SliceToJSBuffer(n.RawData())
	return jslibs.IpfsBlock.New(data, helpers.CidToJSCID(n.Cid()))
}
