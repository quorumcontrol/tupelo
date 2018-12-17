package remote

import (
	gocontext "context"
	"crypto/ecdsa"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	pnet "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/tinylib/msgp/msgp"
)

type internalSetupStreamMessage struct{}
type internalStreamDied struct {
	err error
}

type Bridge struct {
	middleware.LogAwareHolder
	host         p2p.Node
	localAddress string

	remoteKey     *ecdsa.PublicKey
	remoteAddress string

	writer       *msgp.Writer
	stream       pnet.Stream
	streamCtx    gocontext.Context
	streamCancel gocontext.CancelFunc
}

func newBridgeProps(host p2p.Node, remoteKey *ecdsa.PublicKey) *actor.Props {
	peer, err := p2p.PeerFromEcdsaKey(remoteKey)
	if err != nil {
		panic(fmt.Sprintf("error getting peer from key"))
	}

	return actor.FromProducer(func() actor.Actor {
		return &Bridge{
			host:          host,
			localAddress:  NewRoutableAddress(host.Identity(), peer.Pretty()).String(),
			remoteKey:     remoteKey,
			remoteAddress: peer.Pretty(),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (b *Bridge) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case pnet.Stream:
		b.handleIncomingStream(context, msg)
	case *internalSetupStreamMessage:
		b.handleCreateNewStream(context)
	case *internalStreamDied:
		b.handleStreamDied(context, msg)
	case *WireDelivery:
		if msg.Outgoing {
			b.handleOutgoingWireDelivery(context, msg)
		} else {
			b.handleIncomingWireDelivery(context, msg)
		}
	}
}

func (b *Bridge) handleIncomingStream(context actor.Context, stream pnet.Stream) {
	remote := stream.Conn().RemotePeer().Pretty()
	b.Log.Infow("handling incoming stream", "peer", remote)
	if remote != b.remoteAddress {
		b.Log.Infow("ignoring stream from other peer")
	}
	b.clearStream()
	ctx, cancel := gocontext.WithCancel(gocontext.Background())
	b.streamCtx = ctx
	b.streamCancel = cancel
	b.setupNewStream(context, stream)
}

func (b *Bridge) handleIncomingWireDelivery(context actor.Context, wd *WireDelivery) {
	b.Log.Infow("received", "wd", wd)
	var sender *actor.PID
	target := FromActorPid(wd.Target)
	if wd.Sender != nil {
		sender = FromActorPid(wd.Sender)
		sender.Address = routableAddress(sender.Address).Swap().String()
	}
	// switch the target to the local actor system
	target.Address = actor.ProcessRegistry.Address
	msg, err := wd.GetMessage()
	if err != nil {
		panic(fmt.Sprintf("error unmarshaling message: %v", err))
	}
	target.Request(msg, sender)
}

func (b *Bridge) handleOutgoingWireDelivery(context actor.Context, wd *WireDelivery) {
	if b.writer == nil {
		context.Self().Tell(&internalSetupStreamMessage{})
		context.Forward(context.Self())
		return
	}
	b.Log.Infow("writing", "wd", wd)
	if wd.Sender != nil && wd.Sender.Address == actor.ProcessRegistry.Address {
		wd.Sender.Address = b.localAddress
	}
	err := wd.EncodeMsg(b.writer)
	if err != nil {
		panic(fmt.Sprintf("error writing to stream: %v", err))
	}
	b.writer.Flush()
}

func (b *Bridge) handleStreamDied(context actor.Context, msg *internalStreamDied) {
	b.Log.Infow("stream died", "err", msg.err)
	b.clearStream()
}

func (b *Bridge) clearStream() {
	if b.streamCancel != nil {
		b.streamCancel()
	}
	b.stream = nil
	b.writer = nil
	b.streamCtx = nil
}

func (b *Bridge) handleCreateNewStream(context actor.Context) {
	b.Log.Infow("create new stream", "addr", b.remoteAddress)
	ctx, cancel := gocontext.WithCancel(gocontext.Background())
	b.streamCtx = ctx
	b.streamCancel = cancel

	stream, err := b.host.NewStream(ctx, b.remoteKey, p2pProtocol)
	if err != nil {
		panic(fmt.Sprintf("error opening stream: %v", err))
	}
	b.setupNewStream(context, stream)
}

func (b *Bridge) setupNewStream(context actor.Context, stream pnet.Stream) {
	b.stream = stream
	b.writer = msgp.NewWriter(stream)

	go func(ctx gocontext.Context, act *actor.PID, s pnet.Stream) {
		done := ctx.Done()
		reader := msgp.NewReader(stream)
		for {
			select {
			case <-done:
				s.Close()
				return
			default:
				var wd WireDelivery
				err := wd.DecodeMsg(reader)
				if err != nil {
					act.Tell(internalStreamDied{err: err})
				}
				act.Tell(&wd)
			}
		}
	}(b.streamCtx, context.Self(), stream)
}
