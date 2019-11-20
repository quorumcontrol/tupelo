package gossip4

import (
	"context"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionValidator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numMembers := 1
	ts := testnotarygroup.NewTestSet(t, numMembers)
	ng, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, numMembers)

	fut := actor.NewFuture(5 * time.Second)

	validator, err := newTransactionValidator(logging.Logger("txvalidatortest"), ng, fut.PID())
	require.Nil(t, err)

	// an internally consistent Tx is marked as valid and sent to the Node
	abr := testhelpers.NewValidTransaction(t)
	abrBits, err := abr.Marshal()
	require.Nil(t, err)

	isValid := validator.validate(ctx, peer.ID(nodes[0].p2pNode.Identity()), &pubsub.Message{
		Message: &pb.Message{
			Data: abrBits,
		},
	})
	require.True(t, isValid)

	// the Node should receive the ABR (we simulate the node with a future here)
	res, err := fut.Result()
	require.Nil(t, err)
	assert.Equal(t, abr.ObjectId, res.(*services.AddBlockRequest).ObjectId)

	// an abr with a bad new tip should be marked invalid
	abr = testhelpers.NewValidTransaction(t)
	abr.NewTip = []byte{}
	isValid = validator.validateAbr(ctx, &abr)
	require.False(t, isValid)

	// an abr with a bad height should be marked invalid
	abr = testhelpers.NewValidTransaction(t)
	abr.Height = 1000
	isValid = validator.validateAbr(ctx, &abr)
	require.False(t, isValid)
}
