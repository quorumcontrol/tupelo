// +build integration

package gossip2

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

	// f, ferr := os.Create("gossip.prof")
	// if ferr != nil {
	// 	t.Fatal(ferr)
	// }
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	gossipers := generateTestGossipGroup(t, 15, 0)
	for i := 0; i < len(gossipers); i++ {
		gossipers[i].Start()
		defer gossipers[i].Stop()
	}

	message := &GossipMessage{
		ObjectID:    []byte("obj"),
		PreviousTip: nil,
		Transaction: []byte("trans"),
		Phase:       phasePrepare,
		Round:       gossipers[0].roundAt(time.Now()),
	}

	err := gossipers[0].handleGossip(context.TODO(), message)
	require.Nil(t, err)
	now := time.Now()
	for {
		state, err := gossipers[0].getCurrentState(message.ObjectID)
		require.Nil(t, err)
		if state.Tip.Equals(tip(message.Transaction)) {
			break
		}
		<-time.After(100 * time.Millisecond)
		if time.Now().Sub(now) > (20 * time.Second) {
			t.Fatalf("timeout")
			break
		}
	}

}
