package gossip4

import (
	"context"
	"sync"
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

// 25 Nov 2019 - Topper's MBP
// BenchmarkTransactionValidator-12    	    7638	    194252 ns/op	  152368 B/op	    2016 allocs/op
func BenchmarkTransactionValidator(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numMembers := 1
	ts := testnotarygroup.NewTestSet(b, numMembers)
	ng, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(b, err)
	require.Len(b, nodes, numMembers)

	wg := &sync.WaitGroup{}

	receiver := func(actorContext actor.Context) {
		switch actorContext.Message().(type) {
		case *services.AddBlockRequest:
			wg.Done()
		}
	}

	act := actor.EmptyRootContext.Spawn(actor.PropsFromFunc(receiver))
	defer actor.EmptyRootContext.Stop(act)

	validator, err := newTransactionValidator(logging.Logger("txvalidatortest"), ng, act)
	require.Nil(b, err)

	txs := make([]*pubsub.Message, b.N)
	for i := 0; i < b.N; i++ {
		abr := testhelpers.NewValidTransaction(b)
		abrBits, _ := abr.Marshal()
		txs[i] = &pubsub.Message{
			Message: &pb.Message{
				Data: abrBits,
			},
		}
	}

	peerID := peer.ID(nodes[0].p2pNode.Identity())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		// in the real system everything happens in parallel with a max of 1024
		// We don't limit in the benchmark because we're assuming < 1024 TPS
		go func(msg *pubsub.Message) {
			validator.validate(ctx, peerID, msg)
		}(txs[i])
	}
	wg.Wait()
}
