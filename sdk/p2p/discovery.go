package p2p

import (
	"context"
	"fmt"
	"time"

	"sync/atomic"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/AsynkronIT/protoactor-go/eventstream"
	discovery "github.com/libp2p/go-libp2p-discovery"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peerstore"
)

const (
	// EventPeerConnected is emitted to the eventstream
	// whenever a new peer is found
	EventPeerConnected int = iota
)

type tupeloDiscoverer struct {
	namespace  string
	host       *LibP2PHost
	discoverer *discovery.RoutingDiscovery
	connected  uint64
	events     *eventstream.EventStream
	cancelFunc context.CancelFunc
	parentCtx  context.Context
	started    bool
}

type DiscoveryEvent struct {
	Namespace string
	Connected uint64
	EventType int
}

func newTupeloDiscoverer(h *LibP2PHost, namespace string) *tupeloDiscoverer {
	return &tupeloDiscoverer{
		namespace:  namespace,
		host:       h,
		discoverer: discovery.NewRoutingDiscovery(h.routing),
		events:     &eventstream.EventStream{},
	}
}

func (td *tupeloDiscoverer) start(originalContext context.Context) error {
	if td.started {
		return nil
	}
	ctx, cancel := context.WithCancel(originalContext)
	td.cancelFunc = cancel
	td.parentCtx = ctx

	if err := td.constantlyAdvertise(td.parentCtx); err != nil {
		return fmt.Errorf("error advertising: %v", err)
	}
	if err := td.findPeers(td.parentCtx); err != nil {
		return fmt.Errorf("error finding peers: %v", err)
	}
	td.started = true
	return nil
}

func (td *tupeloDiscoverer) stop() {
	if !td.started {
		return
	}
	if td.cancelFunc != nil {
		td.cancelFunc()
		td.parentCtx = nil
		td.cancelFunc = nil
		td.started = false
	}
}

func (td *tupeloDiscoverer) findPeers(ctx context.Context) error {
	log.Debugf("find peers %s", td.namespace)
	peerChan, err := td.discoverer.FindPeers(ctx, td.namespace)
	if err != nil {
		return fmt.Errorf("error findPeers: %v", err)
	}

	go func() {
		for peerInfo := range peerChan {
			td.handleNewPeerInfo(ctx, peerInfo)
		}
	}()
	return nil
}

func (td *tupeloDiscoverer) handleNewPeerInfo(ctx context.Context, p peer.AddrInfo) {
	if p.ID == "" {
		return // empty id
	}

	host := td.host.host

	if host.Network().Connectedness(p.ID) == network.Connected {
		log.Debugf("already connected peer %s", td.namespace)
		numConnected := atomic.AddUint64(&td.connected, uint64(1))
		td.events.Publish(&DiscoveryEvent{
			Namespace: td.namespace,
			Connected: numConnected,
			EventType: EventPeerConnected,
		})
		return // we are already connected
	}

	log.Debugf("new peer: %s", p.ID)

	// do the connection async because connect can hang
	go func() {
		// not actually positive that TTL is correct, but it seemed the most appropriate
		host.Peerstore().AddAddrs(p.ID, p.Addrs, peerstore.ProviderAddrTTL)
		if err := host.Connect(ctx, p); err != nil {
			log.Errorf("error connecting to  %s %v: %v", p.ID, p, err)
		}
		log.Debugf("node connected for namespace %s: %v", td.namespace, p)
		numConnected := atomic.AddUint64(&td.connected, uint64(1))
		td.events.Publish(&DiscoveryEvent{
			Namespace: td.namespace,
			Connected: numConnected,
			EventType: EventPeerConnected,
		})
	}()
}

func (td *tupeloDiscoverer) waitForNumber(num int, duration time.Duration) error {
	doneChan := make(chan struct{})
	sub := td.events.Subscribe(func(evt interface{}) {
		stats, ok := evt.(*DiscoveryEvent)
		if !ok {
			return
		}
		if stats.Connected >= uint64(num) {
			go func() {
				doneChan <- struct{}{}
			}()
		}
	})
	after := time.After(duration)

	currCount := atomic.LoadUint64(&td.connected)
	log.Debugf("currCount (%s) is %d", td.namespace, currCount)
	if currCount >= uint64(num) {
		go func() {
			doneChan <- struct{}{}
		}()
	}

	var err error
	select {
	case <-after:
		err = fmt.Errorf("errror, waiting for number connected (%s)", td.namespace)
	case <-doneChan:
		err = nil
	}
	close(doneChan)
	td.events.Unsubscribe(sub)
	return err
}

func (td *tupeloDiscoverer) constantlyAdvertise(ctx context.Context) error {
	log.Debugf("advertising %s", td.namespace)
	dur, err := td.discoverer.Advertise(ctx, td.namespace)
	if err != nil {

		if err.Error() == "failed to find any peer in table" {
			// if this happened then we just haven't initialized the DHT yet, we can just retry
			time.AfterFunc(2*time.Second, func() {
				log.Infof("(%s) no bootstrap yet, advertising after 2 seconds", td.namespace)
				err = td.constantlyAdvertise(ctx)
				if err != nil {
					log.Errorf("error constantly advertising: %v", err)
				}
			})
			return nil
		}

		return err
	}

	go func() {
		after := time.After(dur)
		select {
		case <-ctx.Done():
			return
		case <-after:
			if err := td.constantlyAdvertise(ctx); err != nil {
				log.Errorf("error constantly advertising: %v", err)
			}
		}
	}()
	return nil
}
