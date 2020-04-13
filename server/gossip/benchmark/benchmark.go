package benchmark

import (
	"bytes"
	"context"
	"fmt"
	"math"
	mathrand "math/rand"
	"sort"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/quorumcontrol/tupelo/sdk/consensus"
	"github.com/quorumcontrol/tupelo/sdk/gossip/client"
	"github.com/quorumcontrol/tupelo/sdk/gossip/hamtwrapper"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
)

type Benchmark struct {
	client        *client.Client
	concurrency   int
	duration      time.Duration
	timeout       time.Duration
	FaultDetector *FaultDetector
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
	trans, err := newBenchmarkTransaction(ctx)
	res := &Result{
		Transaction: trans,
	}
	if err != nil {
		res.Error = err
		resCh <- res
		return
	}
	start := time.Now()
	_, err = b.client.Send(ctx, trans, b.timeout)
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

	roundCh := make(chan *types.RoundConfirmationWrapper)
	roundSubscription, err := b.client.SubscribeToRounds(clientCtx, roundCh)
	if err != nil {
		panic(err)
	}

	delayBetween := time.Duration(float64(time.Second) / float64(b.concurrency))

	resCh := make(chan *Result, b.concurrency*60) // 60 seconds of results buffer

	resultSet := &ResultSet{
		durationSet: &durationSet{},
		Rounds:      &durationSet{},
	}

	go func() {
		var lastTime time.Time
		defer b.client.UnsubscribeFromRounds(roundSubscription)

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
	if b.FaultDetector != nil {
		b.FaultDetector.Stop()
	}
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

func fillerData() string {
	byteSizes := []int{4, 128, 4096, 20480}
	sizeI := mathrand.Int() % len(byteSizes)
	return string(bytes.Repeat([]byte("0"), byteSizes[sizeI]))
}

func newBenchmarkTransaction(ctx context.Context) (*services.AddBlockRequest, error) {
	sw := safewrap.SafeWrap{}

	treeKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	dataTxn, err := chaintree.NewSetDataTransaction("some/data", fillerData())
	if err != nil {
		return nil, err
	}

	docTypeTxn, err := chaintree.NewSetDataTransaction("__doctype", "benchmark")
	if err != nil {
		return nil, err
	}

	unsignedBlock := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{dataTxn, docTypeTxn},
		},
	}

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	nodeStore := nodestore.MustMemoryStore(ctx)
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(ctx, emptyTree, nil, consensus.DefaultTransactors)
	if err != nil {
		return nil, err
	}

	blockWithHeaders, err := consensus.SignBlock(ctx, &unsignedBlock, treeKey)
	if err != nil {
		return nil, err
	}

	valid, err := testTree.ProcessBlock(ctx, blockWithHeaders)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("invalid process block")
	}

	cborNodes, err := testTree.Dag.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}

	bits := sw.WrapObject(blockWithHeaders).RawData()
	if sw.Err != nil {
		return nil, err
	}

	return &services.AddBlockRequest{
		PreviousTip: emptyTip.Bytes(),
		Height:      blockWithHeaders.Height,
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     bits,
		State:       nodes,
		ObjectId:    []byte(treeDID),
	}, nil
}

func abrToHamtCID(ctx context.Context, abr *services.AddBlockRequest) (cid.Cid, error) {
	underlyingStore := nodestore.MustMemoryStore(ctx)
	hamtStore := hamt.CborIpldStore{
		Blocks: hamtwrapper.NewStore(underlyingStore),
	}
	return hamtStore.Put(ctx, abr)
}
