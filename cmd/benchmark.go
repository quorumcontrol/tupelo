package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/messages/build/go/signatures"
	"github.com/quorumcontrol/tupelo/nodebuilder"

	"github.com/ethereum/go-ethereum/crypto"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/tupelo-go-sdk/client"
	gossip3middleware "github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3remote "github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3testhelpers "github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	gossip3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo/resources"
	"github.com/spf13/cobra"
)

type ResultSet struct {
	Durations       []int
	Errors          []string
	Total           int
	Measured        int
	Successes       int
	Failures        int
	AverageDuration int
	MinDuration     int
	MaxDuration     int
	P95Duration     int
}

type Result struct {
	Duration    int
	Error       string
	Transaction *services.AddBlockRequest
}

var results ResultSet
var benchmarkConcurrency int
var benchmarkTimeout int
var benchmarkDuration int
var benchmarkStrategy string
var benchmarkStartDelay int
var tracingTagName string

var sentCh chan *services.AddBlockRequest
var resultsCh chan *Result

func measureTransaction(cli *client.Client, trans *services.AddBlockRequest) {
	result := &Result{
		Transaction: trans,
	}

	subscriptionFuture := cli.Subscribe(trans, time.Duration(benchmarkTimeout)*time.Second)
	defer cli.Stop()

	startTime := time.Now()
	did := string(trans.ObjectId)

	sp := opentracing.StartSpan("benchmark-transaction")
	if tracingTagName != "" {
		sp.SetTag("name", tracingTagName)
	}
	if version, err := resources.Version(); err == nil {
		sp.SetTag("version", version)
	}
	sp.SetTag("chainId", did)
	defer sp.Finish()

	var errMsg string

	// Wait for response
	resp, err := subscriptionFuture.Result()
	if err == nil {
		switch msg := resp.(type) {
		case *signatures.CurrentState:
			elapsed := time.Since(startTime)
			result.Duration = int(elapsed / time.Millisecond)
		case nil:
			result.Error = fmt.Sprintf("%s - nil response from channel", did)
		default:
			result.Error = fmt.Sprintf("%s - unkown error: %v", did, msg)
		}
	} else {
		result.Error = fmt.Sprintf("%s - timeout", did)
	}

	if result.Error != "" {
		sp.SetTag("error", true)
		sp.SetTag("errormessage", errMsg)
	}

	resultsCh <- result
}

func sendTransaction(notaryGroup *types.NotaryGroup, pubsub remote.PubSub, shouldMeasure bool) {
	fakeT := &testing.T{}
	trans := gossip3testhelpers.NewValidTransaction(fakeT)
	cli := client.New(notaryGroup, string(trans.ObjectId), pubsub)
	if shouldMeasure {
		cli.Listen()
		go measureTransaction(cli, &trans)
	}

	err := cli.SendTransaction(&trans)

	if err != nil {
		panic(fmt.Sprintf("Couldn't add transaction, %v", err))
	}
	sentCh <- &trans
}

func performTpsBenchmark(group *gossip3types.NotaryGroup, pubsub remote.PubSub, measureAtNumRemaining int) {
	total := benchmarkDuration * benchmarkConcurrency
	delayBetween := time.Duration(float64(time.Second) / float64(benchmarkConcurrency))

	ticker := time.NewTicker(delayBetween)
	defer ticker.Stop()
	for i := total; i > 0; i-- {
		go sendTransaction(group, pubsub, i <= measureAtNumRemaining)
		<-ticker.C
	}
}

// benchmark represents the shell command
var benchmark = &cobra.Command{
	Use:    "benchmark",
	Short:  "runs a set of operations against a network at specified concurrency",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		log := gossip3middleware.Log

		gossip3remote.Start()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		config := nodebuilderConfig
		if config == nil {
			var err error
			config, err = nodebuilder.LegacyConfig(configNamespace, 0, enableElasticTracing, enableJaegerTracing, overrideKeysFile)
			if err != nil {
				panic(fmt.Errorf("error generating legacy config: %v", err))
			}
		}
		nb := &nodebuilder.NodeBuilder{Config: config}
		nb.StartTracing()

		key, err := crypto.GenerateKey()
		if err != nil {
			panic(fmt.Sprintf("error generating key: %v", err))
		}

		p2pHost, err := nb.BootstrappedP2PNode(ctx, p2p.WithKey(key))
		if err != nil {
			panic(fmt.Sprintf("error setting up p2p host: %v", err))
		}

		gossip3remote.NewRouter(p2pHost)

		group := nb.NotaryGroup()
		group.SetupAllRemoteActors(&key.PublicKey)

		pubSubSystem := remote.NewNetworkPubSub(p2pHost)

		if err = p2pHost.WaitForBootstrap(1+len(group.Signers)/2, 15*time.Second); err != nil {
			panic(err)
		}

		if benchmarkStartDelay > 0 {
			log.Infof("[benchmark] delaying benchmark kickoff for %d seconds...", benchmarkStartDelay)
			time.Sleep(time.Duration(benchmarkStartDelay) * time.Second)
		}

		expectedTotal := benchmarkDuration * benchmarkConcurrency

		sentCh = make(chan *services.AddBlockRequest, expectedTotal)
		defer close(sentCh)

		resultsCh = make(chan *Result, expectedTotal)
		defer close(resultsCh)

		results = ResultSet{}

		var expectedMeasured int

		log.Infof("[benchmark] running benchmark")

		switch benchmarkStrategy {
		case "tps":
			expectedMeasured = benchmarkDuration * benchmarkConcurrency
			go performTpsBenchmark(group, pubSubSystem, expectedMeasured)
		case "tps-no-ack":
			expectedMeasured = benchmarkConcurrency
			go performTpsBenchmark(group, pubSubSystem, expectedMeasured)
		default:
			panic(fmt.Sprintf("Unknown benchmark strategy: %v", benchmarkStrategy))
		}

		for results.Measured < expectedMeasured {
			select {
			case s := <-sentCh:
				log.Infof("[benchmark] request chainID=%s", s.ObjectId)
				results.Total = results.Total + 1
			case r := <-resultsCh:
				log.Infof("[benchmark] results chainID=%s duration=%dms err=%s", r.Transaction.ObjectId, r.Duration, r.Error)
				results.Measured = results.Measured + 1

				if r.Error != "" {
					results.Errors = append(results.Errors, r.Error)
					results.Failures = results.Failures + 1
				} else {
					results.Durations = append(results.Durations, r.Duration)
					results.Successes = results.Successes + 1
				}
			}
		}

		sum := 0
		for _, v := range results.Durations {
			sum = sum + v
		}

		if results.Successes > 0 {
			results.AverageDuration = sum / results.Successes

			sorted := make([]int, len(results.Durations))
			copy(sorted, results.Durations)
			sort.Ints(sorted)

			results.MinDuration = sorted[0]
			results.MaxDuration = sorted[len(sorted)-1]
			p95Index := int64(math.Round(float64(len(sorted))*0.95)) - 1
			results.P95Duration = sorted[p95Index]
		}

		out, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(out))
	},
}

func init() {
	rootCmd.AddCommand(benchmark)
	benchmark.Flags().IntVarP(&benchmarkConcurrency, "concurrency", "c", 1, "how many transactions to execute at once")
	benchmark.Flags().IntVarP(&benchmarkDuration, "iterations", "i", 10, "how many seconds to run benchmark for")
	benchmark.Flags().IntVarP(&benchmarkTimeout, "timeout", "t", 0, "seconds per transaction to wait before timing out")
	benchmark.Flags().IntVarP(&benchmarkStartDelay, "start-delay", "d", 0, "how many seconds to wait before kicking off the benchmark; useful if network needs to stablize first")
	benchmark.Flags().StringVarP(&benchmarkStrategy, "strategy", "s", "", "(tps|tps-no-ack) - tps will measure every transaction, tps-no-ack will only measure the last round of transactions")
	benchmark.Flags().BoolVar(&enableJaegerTracing, "jaeger-tracing", false, "enable jaeger tracing")
	benchmark.Flags().BoolVar(&enableElasticTracing, "elastic-tracing", false, "enable elastic tracing")
	benchmark.Flags().StringVar(&tracingTagName, "tracing-name", "", "adds tag to tracing for lookup")
}
