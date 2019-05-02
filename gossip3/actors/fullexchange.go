package actors

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

const FULL_EXCHANGE_DEFAULT_TIMEOUT = 300 * time.Second

// FullExchange sends all keys in storage to another gossiper
type FullExchange struct {
	middleware.LogAwareHolder
	cfg *FullExchangeConfig
}

type FullExchangeConfig struct {
	ConflictSetRouter *actor.PID
	CurrentStateStore storage.Storage
}

func NewFullExchangeProps(config *FullExchangeConfig) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &FullExchange{
			cfg: config,
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
	gzippedBytes := e.gzipExport()

	if len(gzippedBytes) == 0 {
		return
	}

	payload := &messages.ReceiveFullExchange{
		Payload: gzippedBytes,
	}

	_, err := context.RequestFuture(extmsgs.FromActorPid(msg.Destination), payload, FULL_EXCHANGE_DEFAULT_TIMEOUT).Result()
	if err != nil {
		panic(fmt.Sprintf("exchange with %v failed: %v", msg.Destination, err))
	}
}

func (e *FullExchange) handleReceiveFullExchange(context actor.Context, msg *messages.ReceiveFullExchange) {
	var responseBytes []byte
	e.gzipImport(context, msg.Payload)
	context.Respond(&messages.ReceiveFullExchange{
		Payload: responseBytes,
	})
}

func (e *FullExchange) gzipExport() []byte {
	buf := new(bytes.Buffer)
	w := gzip.NewWriter(buf)
	wroteCount := 0

	e.Log.Debugw("gzipExport started")

	e.cfg.CurrentStateStore.ForEach([]byte{}, func(key, value []byte) error {
		wroteCount++
		prefix := make([]byte, 4)
		binary.BigEndian.PutUint32(prefix, uint32(len(value)))
		w.Write(prefix)
		w.Write(value)
		return nil
	})
	w.Close()

	if wroteCount == 0 {
		return nil
	}

	e.Log.Debugw("gzipExport exported %d keys", wroteCount)

	return buf.Bytes()
}

func (e *FullExchange) gzipImport(context actor.Context, payload []byte) {
	buf := bytes.NewBuffer(payload)
	reader, err := gzip.NewReader(buf)
	if err != nil {
		panic(fmt.Sprintf("Error creating gzip reader %v", err))
	}
	defer reader.Close()

	e.Log.Debugw("gzipImport from %v started", context.Sender())

	bytesLeft := true

	var wroteCount uint64

	for bytesLeft {
		prefix := make([]byte, 4)
		_, err := io.ReadFull(reader, prefix)
		if err != nil {
			panic(fmt.Sprintf("Error reading kv pair length %v", err))
		}
		prefixLength := binary.BigEndian.Uint32(prefix)

		currentStateBits := make([]byte, int(prefixLength))
		_, err = io.ReadFull(reader, currentStateBits)
		if err != nil {
			panic(fmt.Sprintf("Error reading kv pair %v", err))
		}

		var currentState extmsgs.CurrentState
		_, err = currentState.UnmarshalMsg(currentStateBits)
		if err != nil {
			panic(fmt.Errorf("error unmarshaling CurrentState: %v", err))
		}

		context.Send(e.cfg.ConflictSetRouter, &messages.ImportCurrentState{CurrentState: &currentState})

		wroteCount++
		bytesLeft = buf.Len() > 0
	}

	e.Log.Debugw("gzipImport from %v processed %d states", context.Sender(), wroteCount)
}