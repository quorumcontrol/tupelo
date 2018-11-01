package gossip2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestID(t *testing.T) {
	c := ConflictSet{
		ObjectID: []byte("oid"),
		Tip:      []byte("tid"),
	}
	assert.Len(t, c.ID(), 32)
}
