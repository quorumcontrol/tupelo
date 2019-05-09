//go:generate msgp

package messages

import (
	extmsgs "github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
)

func init() {
	extmsgs.RegisterMessage(&RequestCurrentStateSnapshot{})
	extmsgs.RegisterMessage(&ReceiveCurrentStateSnapshot{})
}

type RequestCurrentStateSnapshot struct {
}

func (RequestCurrentStateSnapshot) TypeCode() int8 {
	return -101
}

type ReceiveCurrentStateSnapshot struct {
	Payload []byte
}

func (ReceiveCurrentStateSnapshot) TypeCode() int8 {
	return -102
}
