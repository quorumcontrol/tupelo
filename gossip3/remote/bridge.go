package remote

import (
	gocontext "context"
	"fmt"
	"math/rand"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	pnet "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	peer "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-peer"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/tinylib/msgp/msgp"
)

const maxBridgeBackoffs = 10

type internalStreamDied struct {
	id  uint64
	err error
}

type Bridge struct {
	middleware.LogAwareHolder
	host         p2p.Node
	localAddress string

	remoteAddress peer.ID

	writer       *msgp.Writer
	streamID     uint64
	stream       pnet.Stream
	streamCtx    gocontext.Context
	streamCancel gocontext.CancelFunc
	backoffCount int
}

func newBridgeProps(host p2p.Node, remoteAddress peer.ID) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &Bridge{
			host:          host,
			localAddress:  types.NewRoutableAddress(host.Identity(), remoteAddress.Pretty()).String(),
			remoteAddress: remoteAddress,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

var bridgeReceiveTimeout = 10 * time.Second

func (b *Bridge) Receive(context actor.Context) {
	// defer func() {
	// 	if re := recover(); re != nil {
	// 		b.Log.Errorw("recover", "re", re)
	// 		panic(re)
	// 	}
	// }()
	switch msg := context.Message().(type) {
	case *actor.ReceiveTimeout:
		b.Log.Infow("terminating stream due to lack of activity")
		context.SetBehavior(b.TerminatedState)
		b.clearStream(b.streamID)
		context.Self().Poison()
	case *actor.Terminated:
		b.clearStream(b.streamID)
	case pnet.Stream:
		context.SetReceiveTimeout(bridgeReceiveTimeout)
		b.handleIncomingStream(context, msg)
	case *internalStreamDied:
		b.handleStreamDied(context, msg)
	case *WireDelivery:
		context.SetReceiveTimeout(bridgeReceiveTimeout)
		if msg.Outgoing {
			b.handleOutgoingWireDelivery(context, msg)
		} else {
			b.handleIncomingWireDelivery(context, msg)
		}
	}
}

func (b *Bridge) TerminatedState(context actor.Context) {
	// do nothing
}

func (b *Bridge) handleIncomingStream(context actor.Context, stream pnet.Stream) {
	remote := stream.Conn().RemotePeer()
	b.Log.Debugw("handling incoming stream", "peer", remote)
	if remote != b.remoteAddress {
		b.Log.Errorw("ignoring stream from other peer", "peer", remote)
	}
	b.clearStream(b.streamID)
	ctx, cancel := gocontext.WithCancel(gocontext.Background())
	b.setupNewStream(ctx, cancel, context, stream)
}

func (b *Bridge) handleIncomingWireDelivery(context actor.Context, wd *WireDelivery) {
	// b.Log.Debugw("received", "target", wd.Target, "sender", wd.Sender, "msgHash", crypto.Keccak256(wd.Message))
	var sender *actor.PID
	target := messages.FromActorPid(wd.Target)
	if wd.Sender != nil {
		sender = messages.FromActorPid(wd.Sender)
		sender.Address = types.RoutableAddress(sender.Address).Swap().String()
	}
	// switch the target to the local actor system
	target.Address = actor.ProcessRegistry.Address
	msg, err := wd.GetMessage()
	if err != nil {
		panic(fmt.Sprintf("error unmarshaling message: %v", err))
	}
	dest, ok := msg.(messages.DestinationSettable)
	if ok {
		orig := dest.GetDestination()
		orig.Address = b.localAddress
		dest.SetDestination(orig)
	}
	target.Request(msg, sender)
}

func (b *Bridge) handleOutgoingWireDelivery(context actor.Context, wd *WireDelivery) {
	if b.writer == nil {
		err := b.handleCreateNewStream(context)
		if err != nil {
			b.Log.Infow("error opening stream", "err", err)
			if err == p2p.ErrDialBackoff {
				// back off dialing if we have trouble with the stream
				// non-cryptographic random here to add jitter
				time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)
				b.backoffCount++
				if b.backoffCount > maxBridgeBackoffs {
					context.Self().Stop()
					b.Log.Infow("maximum backoff reached - possibly dropped messages", "count", b.backoffCount)
					return
				}
			}
			context.Forward(context.Self())
			return
		}
	}
	// b.Log.Debugw("writing", "target", wd.Target, "sender", wd.Sender, "msgHash", crypto.Keccak256(wd.Message))
	if wd.Sender != nil && wd.Sender.Address == actor.ProcessRegistry.Address {
		wd.Sender.Address = b.localAddress
	}
	err := wd.EncodeMsg(b.writer)
	if err != nil {
		b.clearStream(b.streamID)
		context.Forward(context.Self())
		return
	}
	b.writer.Flush()
}

func (b *Bridge) handleStreamDied(context actor.Context, msg *internalStreamDied) {
	b.Log.Infow("stream died", "err", msg.err)
	b.clearStream(msg.id)
}

func (b *Bridge) clearStream(id uint64) {
	if b.streamID != id {
		b.Log.Errorw("ids did not match", "expecting", id, "actual", b.streamID)
		return
	}
	if b.stream != nil {
		b.stream.Reset()
	}
	if b.streamCancel != nil {
		b.streamCancel()
	}
	b.stream = nil
	b.writer = nil
	b.streamCtx = nil
}

func (b *Bridge) handleCreateNewStream(context actor.Context) error {
	if b.stream != nil {
		b.Log.Infow("not creating new stream, already exists")
		return nil
	}
	b.Log.Debugw("create new stream")

	ctx, cancel := gocontext.WithCancel(gocontext.Background())

	stream, err := b.host.NewStreamWithPeerID(ctx, b.remoteAddress, p2pProtocol)
	if err != nil {
		b.clearStream(b.streamID)
		cancel()
		return err
	}

	b.setupNewStream(ctx, cancel, context, stream)
	return nil
}

func (b *Bridge) setupNewStream(ctx gocontext.Context, cancelFunc gocontext.CancelFunc, context actor.Context, stream pnet.Stream) {
	b.stream = stream
	b.streamCtx = ctx
	b.streamCancel = cancelFunc
	b.streamID++
	b.writer = msgp.NewWriter(stream)

	go func(ctx gocontext.Context, act *actor.PID, s pnet.Stream, id uint64) {
		done := ctx.Done()
		reader := msgp.NewReader(stream)
		for {
			select {
			case <-done:
				b.Log.Debugw("resetting stream due to done")
				s.Close()
				return
			default:
				var wd WireDelivery
				err := wd.DecodeMsg(reader)
				if err != nil {
					act.Tell(internalStreamDied{id: id, err: err})
					return
				}
				act.Tell(&wd)
			}
		}
	}(b.streamCtx, context.Self(), stream, b.streamID)
}
