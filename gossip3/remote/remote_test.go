package remote

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	pnet "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"
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
	ts := testnotarygroup.NewTestSet(t, 2)
	signer1 := types.NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	// signer2 := types.NewLocalSigner(ts.PubKeys[1].ToEcdsaPub(), ts.SignKeys[1])

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host, err := p2p.NewLibP2PHost(ctx, ts.EcdsaKeys[0], 0)
	require.Nil(t, err)

	Start(signer1, host)
	defer Stop()

	localPing := actor.Spawn(actor.FromFunc(func(ctx actor.Context) {
		switch msg := ctx.Message().(type) {
		case *messages.Ping:
			ctx.Respond(&messages.Pong{Msg: msg.Msg})
		}
	}))
	assert.Equal(t, signer1.ActorAddress(), localPing.Address)

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

	host2.SetStreamHandler(p2pProtocol, func(s pnet.Stream) {
		reader := msgp.NewReader(s)
		writer := msgp.NewWriter(s)

		s.Conn()

		var wd WireDelivery
		err := wd.DecodeMsg(reader)
		require.Nil(t, err)

		var ping messages.Ping
		_, err = ping.UnmarshalMsg(wd.Message)
		require.Nil(t, err)

		pong := &messages.Pong{Msg: ping.Msg}
		bits, err := pong.MarshalMsg(nil)
		require.Nil(t, err)

		response := &WireDelivery{
			Target:  wd.Sender,
			Sender:  nil,
			Message: bits,
			Type:    messages.GetTypeCode(pong),
		}

		response.EncodeMsg(writer)
		writer.Flush()
	})

	// time to bootstrap
	time.Sleep(500 * time.Millisecond)

	Start(signer1, host1)
	defer Stop()

	remotePing := actor.NewPID(signer2.ActorAddress(), "test")
	assert.Equal(t, remotePing.Address, signer2.ActorAddress())

	t.Logf("remotePing: %s, addr: %s, s2Addr: %s", remotePing.String(), remotePing.Address, signer2.ActorAddress())

	_, err = remotePing.RequestFuture(&messages.Ping{Msg: "hi"}, 100*time.Millisecond).Result()
	assert.Equal(t, remotePing.Address, signer2.ActorAddress())

	assert.Nil(t, err)
	// assert.Equal(t, resp.(*pong).msg, "hi")
}
