package gossip3to4

import (
	"io/ioutil"
	"testing"

	cbornode "github.com/ipfs/go-ipld-cbor"
	g3services "github.com/quorumcontrol/messages/build/go/services"
	"github.com/stretchr/testify/require"
)

func TestConvertABR(t *testing.T) {
	g3abrBytes, err := ioutil.ReadFile("testgossip3abr.cbor")
	require.Nil(t, err)

	cbornode.RegisterCborType(g3services.AddBlockRequest{})

	var g3abr g3services.AddBlockRequest
	err = cbornode.DecodeInto(g3abrBytes, &g3abr)
	require.Nil(t, err)

	g4abr, err := ConvertABR(&g3abr)
	require.Nil(t, err)

	require.Equal(t, g3abr.State, g4abr.State)
	require.Equal(t, g3abr.NewTip, g4abr.NewTip)
	require.Equal(t, g3abr.Height, g4abr.Height)
	require.Equal(t, g3abr.PreviousTip, g4abr.PreviousTip)
	require.Equal(t, g3abr.ObjectId, g4abr.ObjectId)

	// Decode and assert some other things on the payload?
	require.NotEqual(t, g3abr.Payload, g4abr.Payload)
}
