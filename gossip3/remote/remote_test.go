package remote

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, error) {
	ng := types.NewNotaryGroup()
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		syncer, err := actor.SpawnNamed(actors.NewTupeloNodeProps(signer, ng), "tupelo-"+signer.ID)
		if err != nil {
			return nil, fmt.Errorf("error spawning: %v", err)
		}
		signer.Actor = syncer
		go func() {
			<-ctx.Done()
			syncer.Poison()
		}()
		ng.AddSigner(signer)
	}
	return ng, nil
}

func newBootstrapHost(ctx context.Context, t *testing.T) p2p.Node {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := p2p.NewLibP2PHost(ctx, key, 0)

	require.Nil(t, err)
	require.NotNil(t, host)

	return host
}

func TestLocalStillWorks(t *testing.T) {
	Start()
	defer Stop()

	localPing := actor.Spawn(actor.FromFunc(func(ctx actor.Context) {
		switch msg := ctx.Message().(type) {
		case *messages.Ping:
			ctx.Respond(&messages.Pong{Msg: msg.Msg})
		}
	}))

	resp, err := localPing.RequestFuture(&messages.Ping{Msg: "hi"}, 1*time.Second).Result()
	require.Nil(t, err)
	assert.Equal(t, resp.(*messages.Pong).Msg, "hi")
}

func TestRemoteMessageSending(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 2)
	signer1 := types.NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	signer2 := types.NewLocalSigner(ts.PubKeys[1].ToEcdsaPub(), ts.SignKeys[1])
	require.NotEmpty(t, signer1.ActorAddress())
	require.NotEmpty(t, signer2.ActorAddress())
	require.NotEqual(t, signer1.ActorAddress(), signer2.ActorAddress())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bootstrap := newBootstrapHost(ctx, t)

	host1, err := p2p.NewLibP2PHost(ctx, ts.EcdsaKeys[0], 0)
	require.Nil(t, err)
	host1.Bootstrap(testnotarygroup.BootstrapAddresses(bootstrap))

	host2, err := p2p.NewLibP2PHost(ctx, ts.EcdsaKeys[1], 0)
	require.Nil(t, err)
	host2.Bootstrap(testnotarygroup.BootstrapAddresses(bootstrap))

	t.Logf("host1: %s / host2: %s", host1.Identity(), host2.Identity())

	pingFunc := func(ctx actor.Context) {
		switch msg := ctx.Message().(type) {
		case *messages.Ping:
			// t.Logf("ctx: %v, msg: %v, sender: %v", ctx, msg, ctx.Sender().Address+ctx.Sender().GetId())
			ctx.Respond(&messages.Pong{Msg: msg.Msg})
		}
	}

	_, err = actor.SpawnNamed(actor.FromFunc(pingFunc), "ping-host1")
	require.Nil(t, err)

	host2Ping, err := actor.SpawnNamed(actor.FromFunc(pingFunc), "ping-host2")
	require.Nil(t, err)

	// time to bootstrap
	time.Sleep(500 * time.Millisecond)

	Start()
	defer Stop()

	// this is the bridge on host1 that sends traffic to signer2
	bridge1 := NewBridge(signer2.DstKey, host1)
	require.NotNil(t, bridge1)

	// this is the bridge on host2 that sends traffic to signer1
	bridge2 := NewBridge(signer1.DstKey, host2)
	require.NotNil(t, bridge2)

	remotePing := actor.NewPID(signer2.ActorAddress(), host2Ping.GetId())
	assert.Equal(t, remotePing.Address, signer2.ActorAddress())

	resp, err := remotePing.RequestFuture(&messages.Ping{Msg: "hi"}, 100*time.Millisecond).Result()
	assert.Equal(t, remotePing.Address, signer2.ActorAddress())

	assert.Nil(t, err)
	assert.Equal(t, resp.(*messages.Pong).Msg, "hi")
}
