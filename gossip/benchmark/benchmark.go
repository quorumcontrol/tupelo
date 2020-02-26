package benchmark

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/client"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/hamtwrapper"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
)

type Benchmark struct {
	client      *client.Client
	concurrency int
	duration    time.Duration
	timeout     time.Duration
}

type durationSet struct {
	Durations       []int
	Total           int64
	AverageDuration int
	MinDuration     int
	MaxDuration     int
	P95Duration     int
}

// Takes the Durations slice and calculates avg/min/max/p95
func (set *durationSet) calculateStats() {
	sum := 0
	for _, v := range set.Durations {
		sum = sum + v
	}

	if sum > 0 {
		set.AverageDuration = sum / len(set.Durations)

		sorted := make([]int, len(set.Durations))
		copy(sorted, set.Durations)
		sort.Ints(sorted)

		set.MinDuration = sorted[0]
		set.MaxDuration = sorted[len(sorted)-1]
		p95Index := int64(math.Round(float64(len(sorted))*0.95)) - 1
		set.P95Duration = sorted[p95Index]
	}
}

type ResultSet struct {
	*durationSet

	FirstRound int64
	Rounds     *durationSet

	Errors    []string
	Measured  int
	Successes int
	Failures  int
}

type Result struct {
	Duration    int
	Error       error
	Transaction *services.AddBlockRequest
}

func NewBenchmark(cli *client.Client, concurrency int, duration time.Duration, timeout time.Duration) *Benchmark {
	return &Benchmark{
		client:      cli,
		concurrency: concurrency,
		duration:    duration,
		timeout:     timeout,
	}
}

func (b *Benchmark) Send(ctx context.Context, resCh chan *Result) {
	trans := testhelpers.NewValidTransaction(&testing.T{})
	res := &Result{
		Transaction: &trans,
	}
	start := time.Now()
	_, err := b.client.Send(ctx, &trans, b.timeout)
	if err != nil {
		res.Error = err
		resCh <- res
		return
	}

	res.Duration = int(time.Since(start) / time.Millisecond)
	resCh <- res
}

func handleResult(resultSet *ResultSet, res *Result) {
	resultSet.Measured++
	if res.Error != nil {
		id, err := abrToHamtCID(context.TODO(), res.Transaction)
		if err != nil {
			panic(fmt.Sprintf("couldn't convert transaction to CID: %v", err))
		}
		resultSet.Errors = append(resultSet.Errors, id.String()+": "+res.Error.Error())
		resultSet.Failures++
		return
	}
	resultSet.Durations = append(resultSet.Durations, res.Duration)
	resultSet.Successes++
}

func (b *Benchmark) Run(ctx context.Context) *ResultSet {
	benchmarkCtx, benchmarkCancel := context.WithTimeout(ctx, b.duration)
	defer benchmarkCancel()

	clientCtx, clientCancel := context.WithCancel(ctx)
	defer clientCancel()

	roundCh := make(chan *types.RoundWrapper)
	roundSubscription, err := b.client.SubscribeToRounds(clientCtx, roundCh)
	if err != nil {
		panic(err)
	}
	defer b.client.UnsubscribeFromRounds(roundSubscription)

	delayBetween := time.Duration(float64(time.Second) / float64(b.concurrency))

	resCh := make(chan *Result, b.concurrency*60) // 60 seconds of results buffer

	resultSet := &ResultSet{
		durationSet: &durationSet{},
		Rounds:      &durationSet{},
	}

	go func() {
		var lastTime time.Time

		for {
			select {
			case <-clientCtx.Done():
				return
			case roundWrapper := <-roundCh:
				resultSet.Rounds.Total++
				if lastTime.IsZero() {
					resultSet.FirstRound = int64(roundWrapper.Height())
				} else {
					resultSet.Rounds.Durations = append(resultSet.Rounds.Durations, int(time.Since(lastTime)/time.Millisecond))
				}
				lastTime = time.Now()
			}
		}
	}()

	go func() {
		for {
			select {
			case <-benchmarkCtx.Done():
				return
			default:
				atomic.AddInt64(&resultSet.Total, int64(1))
				go b.Send(clientCtx, resCh)
				time.Sleep(delayBetween)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-benchmarkCtx.Done():
				return
			case res := <-resCh:
				handleResult(resultSet, res)
			}
		}
	}()

	<-benchmarkCtx.Done()
	// now we wait for "timeout" to happen to make sure we get all the results
	time.Sleep(b.timeout)

	for len(resCh) > 0 {
		handleResult(resultSet, <-resCh)
	}

	resultSet.calculateStats()
	resultSet.Rounds.calculateStats()

	// Just empty this out so results are easier to read
	resultSet.Rounds.Durations = nil

	return resultSet
}

func abrToHamtCID(ctx context.Context, abr *services.AddBlockRequest) (cid.Cid, error) {
	underlyingStore := nodestore.MustMemoryStore(ctx)
	hamtStore := hamt.CborIpldStore{
		Blocks: hamtwrapper.NewStore(underlyingStore),
	}
	return hamtStore.Put(ctx, abr)
}
