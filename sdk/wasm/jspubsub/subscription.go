// +build wasm

package jspubsub

import (
	"context"
	"fmt"
	"syscall/js"

	"github.com/quorumcontrol/tupelo/sdk/gossip/client/pubsubinterfaces"

	"github.com/quorumcontrol/tupelo/sdk/wasm/helpers"

	pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

type BridgedSubscription struct {
	pubsubinterfaces.Subscription

	pubsub *PubSubBridge
	jsFunc js.Func
	topic  string
	ch     chan pubsubinterfaces.Message
}

func newBridgedSubscription(topic string) *BridgedSubscription {
	return &BridgedSubscription{
		topic: topic,
		ch:    make(chan pubsubinterfaces.Message),
	}
}

func (bs *BridgedSubscription) Next(ctx context.Context) (pubsubinterfaces.Message, error) {
	done := ctx.Done()
	select {
	case <-done:
		return nil, fmt.Errorf("context done")
	case msg := <-bs.ch:
		return msg, nil
	}
}

func (bs *BridgedSubscription) Cancel() {
	go bs.pubsub.jspubsub.Call("unsubscribe", js.ValueOf(bs.topic), bs.jsFunc)
	bs.jsFunc.Release()
}

func (bs *BridgedSubscription) QueueJS(msg js.Value) {
	// js looks like:
	// {
	//     from: 'QmSWBdQGuX8Uregx8QSKxCxk1tacQPgJqX1AXnSUnzDyEM',
	//     data: <Buffer 68 69>,
	//     seqno: <Buffer fe 54 4a ad 49 b4 94 5d 09 98 ad 0e ad 70 43 33 4c fc 4f a5>,
	//     topicIDs: [ 'test' ]
	//   }
	pubsubMsg := &pb.Message{
		From:     []byte(msg.Get("from").String()),
		Seqno:    helpers.JsBufferToBytes(msg.Get("seqno")),
		Data:     helpers.JsBufferToBytes(msg.Get("data")),
		TopicIDs: helpers.JsStringArrayToStringSlice(msg.Get("topicIDs")),
	}

	bs.ch <- pubsubMsg

}
