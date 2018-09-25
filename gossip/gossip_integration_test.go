// +build integration

package gossip

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
)

func TestGossiper_Integration(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	// This bit of commented out code will run the CPU profiler
	// f, ferr := os.Create("gossip.prof")
	// if ferr != nil {
	// 	t.Fatal(ferr)
	// }
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	numberOfNodes := 20

	gossipers := generateTestGossipGroup(t, numberOfNodes, 0)
	for i := 0; i < len(gossipers); i++ {
		gossipers[i].Start()
		defer gossipers[i].Stop()
	}

	message := &GossipMessage{
		ObjectID:    []byte("obj"),
		PreviousTip: nil,
		Transaction: []byte("trans"),
		Phase:       phasePrepare,
		Round:       gossipers[0].RoundAt(time.Now()),
	}

	for i := 0; i < gossipers[0].Fanout; i++ {
		go func() {
			err := gossipers[i].HandleGossip(context.TODO(), message)
			require.Nil(t, err)
		}()
	}

	now := time.Now()
	for {
		state, err := gossipers[0].GetCurrentState(message.ObjectID)
		require.Nil(t, err)
		if state.Tip.Equals(tip(message.Transaction)) {
			break
		}
		time.Sleep(200 * time.Millisecond)
		if time.Now().Sub(now) > (20 * time.Second) {
			t.Fatalf("timeout on commit")
			break
		}
	}
	// assert that the network was saturated
	now = time.Now()
	for {
		count := 0
		for _, gossiper := range gossipers {
			state, err := gossiper.GetCurrentState(message.ObjectID)
			require.Nil(t, err)
			if state.Tip.Equals(tip(message.Transaction)) {
				count++
			}
		}
		if count >= len(gossipers) {
			break
		}
		time.Sleep(200 * time.Millisecond)
		if time.Now().Sub(now) > (10 * time.Second) {
			t.Fatalf("timeout on saturation")
			break
		}
	}
}
