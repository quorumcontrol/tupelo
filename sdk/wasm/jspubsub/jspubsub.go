// +build wasm

package jspubsub

import (
	"fmt"
	"syscall/js"

	"github.com/quorumcontrol/tupelo/sdk/gossip/client/pubsubinterfaces"

	"github.com/quorumcontrol/tupelo/sdk/wasm/helpers"
)

// PubSubBridge is a bridge where golang (in wasm) can use the underlying javascript pubsub
// for (for example) the tupelo client
type PubSubBridge struct {
	pubsubinterfaces.Pubsubber
	jspubsub js.Value
}

func NewPubSubBridge(jspubsub js.Value) *PubSubBridge {
	return &PubSubBridge{
		jspubsub: jspubsub,
	}
}

func (psb *PubSubBridge) Publish(topic string, data []byte) error {
	resp := make(chan error)

	onError := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		resp <- fmt.Errorf("error publishing: %s", args[0].String())
		return nil
	})

	onSuccess := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		resp <- nil
		return nil
	})

	defer func() {
		onSuccess.Release()
		onError.Release()
		close(resp)
	}()

	go func() {
		promise := psb.jspubsub.Call("publish", js.ValueOf(topic), helpers.SliceToJSBuffer(data))
		promise.Call("then", onSuccess, onError)
	}()

	return <-resp
}

func (psb *PubSubBridge) Subscribe(topic string) (pubsubinterfaces.Subscription, error) {
	sub := newBridgedSubscription(topic)
	sub.pubsub = psb
	resp := make(chan error)
	onError := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		resp <- fmt.Errorf("error subscribing: %s", args[0].String())
		return nil
	})

	onSuccess := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
		resp <- nil
		return nil
	})

	defer func() {
		onSuccess.Release()
		onError.Release()
		close(resp)
	}()

	go func() {
		subFunc := js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
			go func() {
				sub.QueueJS(args[0])
			}()
			return nil
		})
		sub.jsFunc = subFunc
		promise := psb.jspubsub.Call("subscribe", js.ValueOf(topic), subFunc)
		promise.Call("then", onSuccess, onError)
	}()
	err := <-resp
	if err != nil {
		return nil, err
	}
	return sub, nil
}
