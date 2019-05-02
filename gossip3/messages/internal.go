//go:generate msgp

package messages

import (
	extmsgs "github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
)

func init() {
	extmsgs.RegisterMessage(&RequestFullExchange{})
	extmsgs.RegisterMessage(&ReceiveFullExchange{})
}

type RequestFullExchange struct {
	Destination *extmsgs.ActorPID
}

func (RequestFullExchange) TypeCode() int8 {
	return -101
}

type ReceiveFullExchange struct {
	Payload             []byte
	RequestExchangeBack bool
}

func (ReceiveFullExchange) TypeCode() int8 {
	return -102
}
