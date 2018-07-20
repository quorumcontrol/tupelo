// +build integration

package gossip

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/qc3/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGossiper_Start(t *testing.T) {
	//log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	gossipers := generateTestGossipGroup(t, 20, 0)
	for i := 0; i < len(gossipers); i++ {
		gossipers[i].Start()
		defer gossipers[i].Stop()
	}

	message := &GossipMessage{
		ObjectId:    []byte("obj"),
		Transaction: []byte("trans"),
	}

	req, err := network.BuildRequest(MessageType_Gossip, message)
	assert.Nil(t, err)

	log.Debug("submitting initial to", "g", gossipers[0].Id)

	//f, err := os.Create("gossip.prof")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//pprof.StartCPUProfile(f)
	//defer pprof.StopCPUProfile()

	resp, err := gossipers[0].HandleGossipRequest(context.TODO(), *req)
	assert.Nil(t, err)

	gossipResp := &GossipMessage{}
	err = cbornode.DecodeInto(resp.Payload, gossipResp)
	assert.Nil(t, err)

	assert.Len(t, gossipResp.Signatures, 1)

	now := time.Now()
	for {
		state, err := gossipers[0].getCurrentState(message.ObjectId)
		require.Nil(t, err)
		if bytes.Equal(state, message.Transaction) {
			break
		}
		<-time.After(100 * time.Millisecond)
		if time.Now().Sub(now) > (20 * time.Second) {
			sigs, _ := gossipers[0].savedSignaturesFor(context.Background(), message.Id())
			t.Fatalf("timeout. State: %v, SigCount: %v", string(state), len(sigs))
			break
		}
	}

	count := 0
	for i := 0; i < len(gossipers); i++ {
		isDone, err := gossipers[i].IsTransactionAccepted(message.Id())
		if err != nil {
			t.Fatalf("error getting accepted: %v", err)
		}
		if isDone {
			count++
		}
	}

	// The original gossiper should have added the other gossiper
	sigs, err := gossipers[0].savedSignaturesFor(context.Background(), message.Id())
	assert.Nil(t, err)
	assert.True(t, int64(len(sigs)) > gossipers[0].Group.SuperMajorityCount())
}
