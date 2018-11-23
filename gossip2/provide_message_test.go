package gossip2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageTypeFromKey(t *testing.T) {
	for _, test := range []struct {
		Key          []byte
		ExpectedType MessageType
	}{
		{
			Key:          append(objectPrefix, []byte("whatever")...),
			ExpectedType: MessageTypeCurrentState,
		},
		{
			Key:          (&Transaction{}).StoredID(),
			ExpectedType: MessageTypeTransaction,
		},
		{
			Key:          (&Signature{}).StoredID([]byte("conflictsetIdDon'tMatter")),
			ExpectedType: MessageTypeSignature,
		},
		{
			Key:          doneIDFromConflictSetID([]byte("conflictsetdon'tmatter")),
			ExpectedType: MessageTypeDone,
		},
	} {
		assert.Equalf(t, test.ExpectedType, messageTypeFromKey(test.Key), "Key %s type != %s", test.Key, test.ExpectedType.string())
	}
}
