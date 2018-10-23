package gossip2

import (
	"testing"

	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitySizes(t *testing.T) {
	filter := ibf.NewInvertibleBloomFilter(2000, 4)
	objectID := []byte("himynameisalongobjectidthatwillhavemorethan64bits")
	id := byteToIBFsObjectId(objectID)
	filter.Add(id)
	data, err := filter.MarshalMsg(nil)
	require.Nil(t, err)
	assert.True(t, len(data) < 50000)
}
