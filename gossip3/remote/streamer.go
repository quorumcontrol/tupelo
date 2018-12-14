package remote

import (
	gocontext "context"
	"crypto/ecdsa"
	"fmt"
	"reflect"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	pnet "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/tinylib/msgp/msgp"
)

const p2pProtocol = "remoteactors/v1.0"

type streamHolder map[string]*actor.PID

type streamActor struct {
	address   string
	remoteKey *ecdsa.PublicKey
	middleware.LogAwareHolder
	streamCtx gocontext.Context
	cancel    gocontext.CancelFunc
	host      p2p.Node
	writer    *msgp.Writer
	stream    pnet.Stream
}

func keyFromAddr(addr string) *ecdsa.PublicKey {
	bits, err := hexutil.Decode(addr)
	if err != nil {
		panic(fmt.Sprintf("error decoding addr: %v", err))
	}
	return crypto.ToECDSAPub(bits)
}

func (sa *streamActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Terminated:
		if sa.cancel != nil {
			sa.cancel()
		}
	case *internalSetupStreamMessage:
		sa.Log.Infow("setting up stream", "addr", sa.address)
		ctx, cancel := gocontext.WithCancel(gocontext.Background())
		sa.streamCtx = ctx
		sa.cancel = cancel
		stream, err := sa.host.NewStream(ctx, sa.remoteKey, p2pProtocol)
		if err != nil {
			panic(fmt.Sprintf("error opening stream: %v", err))
		}
		sa.stream = stream
		sa.writer = msgp.NewWriter(stream)
		go func(ctx gocontext.Context, act *actor.PID, s pnet.Stream) {
			done := ctx.Done()
			reader := msgp.NewReader(stream)
			for {
				select {
				case <-done:
					// just let it die
					return
				default:
					var wd WireDelivery
					err := wd.DecodeMsg(reader)
					if err != nil {
						act.Tell(internalStreamDied{})
					}
					act.Tell(&wd)
				}
			}
		}(ctx, context.Self(), stream)
		context.Self().Tell(msg.Wd)
	case *WireDelivery:
		sa.Log.Infow("wire delivery", "wd", msg)
		if msg.Outgoing {
			if sa.stream == nil {
				context.Self().Tell(&internalSetupStreamMessage{
					Wd: msg,
				})
				return
			}
			if sa.writer == nil {
				panic(fmt.Sprintf("unknown writer"))
			}
			err := msg.EncodeMsg(sa.writer)
			if err != nil {
				panic(fmt.Sprintf("error writing to stream: %v", err))
			}
			sa.writer.Flush()
		} else {
			context.Parent().Tell(msg)
		}

	}
}

func newStreamActorProps(host p2p.Node, address string) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &streamActor{
			address:   address,
			remoteKey: keyFromAddr(address),
			host:      host,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

type internalSetupStreamMessage struct {
	Wd *WireDelivery
}
type internalStreamDied struct{}

type streamer struct {
	middleware.LogAwareHolder
	streams streamHolder
	host    p2p.Node
}

func newStreamerProps(host p2p.Node) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &streamer{
			streams: make(streamHolder),
			host:    host,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (s *streamer) Receive(context actor.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.Log.Errorw("recover", "r", r)
		}
	}()
	switch msg := context.Message().(type) {
	case pnet.Stream:
		s.Log.Infow("new stream", "stream", s)

	case *WireDelivery:
		s.Log.Infow("wire delivery", "msg", msg)

		if msg.Outgoing {
			stream, ok := s.streams[msg.Sender.Address]
			if !ok {
				s.Log.Infof("spawning a new stream")
				st, err := context.SpawnNamed(newStreamActorProps(s.host, msg.Target.Address), "stream-"+msg.Target.Address)
				if err != nil {
					panic(fmt.Sprintf("error spawning: %v", err))
				}
				s.streams[msg.Sender.Address] = st
				stream = st
			}
			stream.Tell(msg)
		} else {
			s.Log.Infow("would deliver")
			var sender *actor.PID
			var target *actor.PID
			if msg.Sender != nil {
				sender = FromActorPid(msg.Sender)
			}
			target = FromActorPid(msg.Target)
			obj := messages.GetEncodable(msg.Type)
			s.Log.Infow("obj", "obj", obj, "type", reflect.TypeOf(obj))
			_, err := obj.UnmarshalMsg(msg.Message)
			if err != nil {
				panic(fmt.Sprintf("error unmarshaling: %v", err))
			}
			// TODO: only allow self as target
			target.Request(obj, sender)
		}
	}
}
