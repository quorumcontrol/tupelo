package remote

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/storage"
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
		syncer, err := actor.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
			Self:              signer,
			NotaryGroup:       ng,
			CommitStore:       storage.NewMemStorage(),
			CurrentStateStore: storage.NewMemStorage(),
		}), "tupelo-"+signer.ID)
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
	ts := testnotarygroup.NewTestSet(t, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bootstrap := newBootstrapHost(ctx, t)

	host1, err := p2p.NewLibP2PHost(ctx, ts.EcdsaKeys[0], 0)
	require.Nil(t, err)
	host1.Bootstrap(testnotarygroup.BootstrapAddresses(bootstrap))

	host2, err := p2p.NewLibP2PHost(ctx, ts.EcdsaKeys[1], 0)
	require.Nil(t, err)
	host2.Bootstrap(testnotarygroup.BootstrapAddresses(bootstrap))

	host3, err := p2p.NewLibP2PHost(ctx, ts.EcdsaKeys[2], 0)
	require.Nil(t, err)
	host3.Bootstrap(testnotarygroup.BootstrapAddresses(bootstrap))

	// time to bootstrap
	time.Sleep(500 * time.Millisecond)

	t.Logf("host1: %s / host2: %s / host3: %s", host1.Identity(), host2.Identity(), host3.Identity())

	pingFunc := func(ctx actor.Context) {
		switch msg := ctx.Message().(type) {
		case *messages.Ping:
			// t.Logf("ctx: %v, msg: %v, sender: %v", ctx, msg, ctx.Sender().Address+ctx.Sender().GetId())
			ctx.Respond(&messages.Pong{Msg: msg.Msg})
		}
	}

	_, err = actor.SpawnNamed(actor.FromFunc(pingFunc), "ping-host1")
	require.Nil(t, err)

	_, err = actor.SpawnNamed(actor.FromFunc(pingFunc), "ping-host2")
	require.Nil(t, err)

	host3Ping, err := actor.SpawnNamed(actor.FromFunc(pingFunc), "ping-host3")
	require.Nil(t, err)

	Start()
	defer Stop()

	NewRouter(host1)
	NewRouter(host2)
	NewRouter(host3)

	RegisterBridge(host1.Identity(), host2.PublicKey())
	RegisterBridge(host1.Identity(), host3.PublicKey())

	RegisterBridge(host2.Identity(), host1.PublicKey())
	RegisterBridge(host2.Identity(), host3.PublicKey())

	RegisterBridge(host3.Identity(), host1.PublicKey())
	RegisterBridge(host3.Identity(), host2.PublicKey())

	remotePing := actor.NewPID(types.NewRoutableAddress(host1.Identity(), host3.Identity()).String(), host3Ping.GetId())

	resp, err := remotePing.RequestFuture(&messages.Ping{Msg: "hi"}, 100*time.Millisecond).Result()

	assert.Nil(t, err)
	assert.Equal(t, resp.(*messages.Pong).Msg, "hi")
}
