package gossip2

import "bytes"

type MessageType int

const (
	MessageTypeSignature MessageType = iota + 1
	MessageTypeTransaction
	MessageTypeDone
	MessageTypeCurrentState
)

func messageTypeFromKey(key []byte) MessageType {
	if bytes.Equal(key[0:len(objectPrefix)], objectPrefix) {
		return MessageTypeCurrentState
	}
	return MessageType(key[8])
}

func (msg *ProvideMessage) Type() MessageType {
	return messageTypeFromKey(msg.Key)
}

func (mt MessageType) string() string {
	switch mt {
	case MessageTypeSignature:
		return "signature"
	case MessageTypeTransaction:
		return "transaction"
	case MessageTypeDone:
		return "done"
	}
	return "unknown"
}
