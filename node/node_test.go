package node_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/gogo/protobuf/proto"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
)

func TestAnyToObj(t *testing.T) {
	assert.Equal(t, "consensuspb.SignatureRequest", proto.MessageName(&consensuspb.SignatureRequest{}))
}
