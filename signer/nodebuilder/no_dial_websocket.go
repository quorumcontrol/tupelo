package nodebuilder

import (
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	ws "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
)

// NoDialWebSocketTransport is a thin wrapper around the websocket transport
// but does not allow actually *dialing* the peer
type NoDialWebSocketTransport struct {
	*ws.WebsocketTransport
}

// NewNoDialWebsocketTransport wraps the standard go-ws-transport New to return the non-dialing version
func NewNoDialWebsocketTransport(u *tptu.Upgrader) *NoDialWebSocketTransport {
	wsTransport := ws.New(u)
	return &NoDialWebSocketTransport{
		WebsocketTransport: wsTransport,
	}
}

// CanDial always returns false because for this transport we want to allow connections
// but *never* want to dial out to the websockets
func (ndwst *NoDialWebSocketTransport) CanDial(a ma.Multiaddr) bool {
	return false
}
