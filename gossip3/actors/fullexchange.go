package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

const FULL_EXCHANGE_DEFAULT_TIMEOUT = 300 * time.Second

// FullExchange sends all keys in storage to another gossiper
type FullExchange struct {
	middleware.LogAwareHolder
	storageActor *actor.PID
}

func NewFullExchangeProps(storageActor *actor.PID) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &FullExchange{
			storageActor: storageActor,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (e *FullExchange) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.RequestFullExchange:
		e.handleRequestFullExchange(context, msg)
	case *messages.ReceiveFullExchange:
		e.handleReceiveFullExchange(context, msg)
	}
}

func (e *FullExchange) handleRequestFullExchange(context actor.Context, msg *messages.RequestFullExchange) {
	gzippedBytes, err := e.gzipExport(context)

	if err != nil {
		panic(err)
	}

	if len(gzippedBytes) == 0 {
		return
	}

	payload := &messages.ReceiveFullExchange{
		Payload:             gzippedBytes,
		RequestExchangeBack: true,
	}

	response, err := context.RequestFuture(extmsgs.FromActorPid(msg.Destination), payload, FULL_EXCHANGE_DEFAULT_TIMEOUT).Result()
	if err != nil {
		panic(fmt.Sprintf("exchange with %v failed: %v", msg.Destination, err))
	}

	responseBytes := response.(*messages.ReceiveFullExchange).Payload

	if len(responseBytes) > 0 {
		context.RequestFuture(e.storageActor, &messages.GzipImport{Payload: responseBytes}, FULL_EXCHANGE_DEFAULT_TIMEOUT).Wait()
	}

	context.Respond(true)
}

func (e *FullExchange) handleReceiveFullExchange(context actor.Context, msg *messages.ReceiveFullExchange) {
	var responseBytes []byte
	var err error

	if msg.RequestExchangeBack {
		responseBytes, err = e.gzipExport(context)
	}

	context.RequestFuture(e.storageActor, &messages.GzipImport{Payload: msg.Payload}, FULL_EXCHANGE_DEFAULT_TIMEOUT).Wait()

	if err != nil {
		panic(fmt.Sprintf("error generating return exchange %v", err))
	}

	context.Respond(&messages.ReceiveFullExchange{
		Payload: responseBytes,
	})
}

func (e *FullExchange) gzipExport(context actor.Context) ([]byte, error) {
	gzipped, err := context.RequestFuture(e.storageActor, &messages.GzipExport{}, FULL_EXCHANGE_DEFAULT_TIMEOUT).Result()
	return gzipped.([]byte), err
}
