package remote

// import (
// 	gocontext "context"
// 	"crypto/ecdsa"
// 	"fmt"

// 	"github.com/AsynkronIT/protoactor-go/actor"
// 	"github.com/AsynkronIT/protoactor-go/plugin"
// 	"github.com/ethereum/go-ethereum/common/hexutil"
// 	"github.com/ethereum/go-ethereum/crypto"
// 	pnet "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
// 	"github.com/quorumcontrol/tupelo/gossip3/middleware"
// 	"github.com/quorumcontrol/tupelo/p2p"
// 	"github.com/tinylib/msgp/msgp"
// )

// type streamActor struct {
// 	address   string
// 	remoteKey *ecdsa.PublicKey
// 	middleware.LogAwareHolder
// 	streamCtx gocontext.Context
// 	cancel    gocontext.CancelFunc
// 	host      p2p.Node
// 	writer    *msgp.Writer
// 	stream    pnet.Stream
// }

// func keyFromAddr(addr string) *ecdsa.PublicKey {
// 	bits, err := hexutil.Decode(addr)
// 	if err != nil {
// 		panic(fmt.Sprintf("error decoding addr: %v", err))
// 	}
// 	return crypto.ToECDSAPub(bits)
// }

// func (sa *streamActor) Receive(context actor.Context) {
// 	switch msg := context.Message().(type) {
// 	case *actor.Terminated:
// 		if sa.cancel != nil {
// 			sa.cancel()
// 		}
// 	case *internalSetupStreamMessage:
// 		sa.Log.Infow("setting up stream", "addr", sa.address)
// 		ctx, cancel := gocontext.WithCancel(gocontext.Background())
// 		sa.streamCtx = ctx
// 		sa.cancel = cancel
// 		stream, err := sa.host.NewStream(ctx, sa.remoteKey, p2pProtocol)
// 		if err != nil {
// 			panic(fmt.Sprintf("error opening stream: %v", err))
// 		}
// 		sa.stream = stream
// 		sa.writer = msgp.NewWriter(stream)
// 		go func(ctx gocontext.Context, act *actor.PID, s pnet.Stream) {
// 			done := ctx.Done()
// 			reader := msgp.NewReader(stream)
// 			for {
// 				select {
// 				case <-done:
// 					// just let it die
// 					return
// 				default:
// 					var wd WireDelivery
// 					err := wd.DecodeMsg(reader)
// 					if err != nil {
// 						act.Tell(internalStreamDied{})
// 					}
// 					act.Tell(&wd)
// 				}
// 			}
// 		}(ctx, context.Self(), stream)
// 		context.Self().Tell(msg.Wd)
// 	case *WireDelivery:
// 		sa.Log.Infow("wire delivery", "wd", msg)
// 		if msg.Outgoing {
// 			if sa.stream == nil {
// 				context.Self().Tell(&internalSetupStreamMessage{
// 					Wd: msg,
// 				})
// 				return
// 			}
// 			if sa.writer == nil {
// 				panic(fmt.Sprintf("unknown writer"))
// 			}
// 			err := msg.EncodeMsg(sa.writer)
// 			if err != nil {
// 				panic(fmt.Sprintf("error writing to stream: %v", err))
// 			}
// 			sa.writer.Flush()
// 		} else {
// 			context.Parent().Tell(msg)
// 		}

// 	}
// }

// func newStreamActorProps(host p2p.Node, address string) *actor.Props {
// 	return actor.FromProducer(func() actor.Actor {
// 		return &streamActor{
// 			address:   address,
// 			remoteKey: keyFromAddr(address),
// 			host:      host,
// 		}
// 	}).WithMiddleware(
// 		middleware.LoggingMiddleware,
// 		plugin.Use(&middleware.LogPlugin{}),
// 	)
// }

// type internalSetupStreamMessage struct {
// 	Wd *WireDelivery
// }
// type internalStreamDied struct{}
