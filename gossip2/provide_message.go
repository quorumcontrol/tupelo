package gossip2

type MessageType int

const (
	MessageTypeSignature MessageType = iota + 1
	MessageTypeTransaction
	MessageTypeDone
)

func messageTypeFromKey(key []byte) MessageType {
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
