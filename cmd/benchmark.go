package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/tupelo-go-sdk/client"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3remote "github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3testhelpers "github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	gossip3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo-go-sdk/tracing"
	"github.com/quorumcontrol/tupelo/resources"
	"github.com/spf13/cobra"
)

type ResultSet struct {
	Successes       int
	Failures        int
	Durations       []int
	Errors          []string
	AverageDuration int
	MinDuration     int
	MaxDuration     int
	P95Duration     int
}

var results ResultSet
var benchmarkConcurrency int
var benchmarkIterations int
var benchmarkTimeout int
var benchmarkStrategy string
var benchmarkSignersFanoutNumber int
var benchmarkStartDelay int
var tracingTagName string

var activeCounter = 0

func measureTransaction(cli *client.Client, trans messages.Transaction) {
	subscriptionFuture := cli.Subscribe(&trans, 30*time.Second)
	defer cli.Stop()

	startTime := time.Now()
	did := string(trans.ObjectID)

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
		case *messages.CurrentState:
			elapsed := time.Since(startTime)
			duration := int(elapsed / time.Millisecond)
			results.Durations = append(results.Durations, duration)
			results.Successes = results.Successes + 1
		case *messages.Error:
			errMsg = fmt.Sprintf("%s - error %d, %v", did, msg.Code, msg.Memo)
		case nil:
			errMsg = fmt.Sprintf("%s - nil response from channel", did)
		default:
			errMsg = fmt.Sprintf("%s - unkown error: %v", did, resp)
		}
	} else {
		errMsg = fmt.Sprintf("%s - timeout", did)
	}

	if errMsg != "" {
		sp.SetTag("error", true)
		sp.SetTag("errormessage", errMsg)
		results.Errors = append(results.Errors, errMsg)
		results.Failures = results.Failures + 1
	}

	activeCounter--
}

func sendTransaction(notaryGroup *types.NotaryGroup, pubsub remote.PubSub, shouldMeasure bool) {
	fakeT := &testing.T{}
	trans := gossip3testhelpers.NewValidTransaction(fakeT)
	cli := client.New(notaryGroup, string(trans.ObjectID), pubsub)
	if shouldMeasure {
		cli.Listen()
		go measureTransaction(cli, trans)
	}

	err := cli.SendTransaction(&trans)
	if err != nil {
		panic(fmt.Sprintf("Couldn't add transaction, %v", err))
	}

	activeCounter++
}

func performTpsBenchmark(group *gossip3types.NotaryGroup, pubsub remote.PubSub) {
	for benchmarkIterations > 0 {
		for i2 := 1; i2 <= benchmarkConcurrency; i2++ {
			go sendTransaction(group, pubsub, true)
		}
		benchmarkIterations--
		time.Sleep(1 * time.Second)
	}
}

func performTpsNoAckBenchmark(group *gossip3types.NotaryGroup, pubsub remote.PubSub) {
	for benchmarkIterations > 0 {
		for i2 := 1; i2 <= benchmarkConcurrency; i2++ {
			// measure only the final iteration
			if benchmarkIterations <= 1 {
				go sendTransaction(group, pubsub, true)
			} else {
				go sendTransaction(group, pubsub, false)
			}
		}
		benchmarkIterations--
		time.Sleep(1 * time.Second)
	}
}

func performLoadBenchmark(group *gossip3types.NotaryGroup, pubsub remote.PubSub) {
	for benchmarkIterations > 0 {
		if activeCounter < benchmarkConcurrency {
			sendTransaction(group, pubsub, true)
			benchmarkIterations--
		}
		time.Sleep(30 * time.Millisecond)
	}
}

// benchmark represents the shell command
var benchmark = &cobra.Command{
	Use:    "benchmark",
	Short:  "runs a set of operations against a network at specified concurrency",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		gossip3remote.Start()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if enableElasticTracing && enableJaegerTracing {
			panic("only one tracing library may be used at once")
		}
		if enableJaegerTracing {
			tracing.StartJaeger("benchmarker")
			defer tracing.StopJaeger()
		}
		if enableElasticTracing {
			tracing.StartElastic()
		}

		key, err := crypto.GenerateKey()
		if err != nil {
			panic(fmt.Sprintf("error generating key: %v", err))
		}

		p2pHost, err := p2p.NewLibP2PHost(ctx, key, 0)
		if err != nil {
			panic(fmt.Sprintf("error setting up p2p host: %v", err))
		}

		if _, err = p2pHost.Bootstrap(p2p.BootstrapNodes()); err != nil {
			panic(err)
		}

		gossip3remote.NewRouter(p2pHost)

		group := setupNotaryGroup(nil, bootstrapPublicKeys)
		group.SetupAllRemoteActors(&key.PublicKey)

		pubSubSystem := remote.NewNetworkPubSub(p2pHost)

		if err = p2pHost.WaitForBootstrap(1+len(group.Signers)/2, 15*time.Second); err != nil {
			panic(err)
		}

		results = ResultSet{}

		doneCh := make(chan bool, 1)
		defer close(doneCh)

		if benchmarkStartDelay > 0 {
			fmt.Printf("Delaying benchmark kickoff for %d seconds...\n", benchmarkStartDelay)
			time.Sleep(time.Duration(benchmarkStartDelay) * time.Second)
			fmt.Println("Running benchmark")
		}

		switch benchmarkStrategy {
		case "tps":
			go performTpsBenchmark(group, pubSubSystem)
		case "tps-no-ack":
			go performTpsNoAckBenchmark(group, pubSubSystem)
		case "load":
			go performLoadBenchmark(group, pubSubSystem)
		default:
			panic(fmt.Sprintf("Unknown benchmark strategy: %v", benchmarkStrategy))
		}

		// Wait to call done until all transactions have finished
		go func() {
			for benchmarkIterations > 0 || activeCounter > 0 {
				time.Sleep(1 * time.Second)
			}
			doneCh <- true
		}()

		if benchmarkTimeout > 0 {
			select {
			case <-doneCh:
			case <-time.After(time.Duration(benchmarkTimeout) * time.Second):
				results.Errors = append(results.Errors, "WARNING: timeout was triggered")
			}
		} else {
			<-doneCh
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
	benchmark.Flags().IntVarP(&benchmarkIterations, "iterations", "i", 10, "how many transactions to execute total")
	benchmark.Flags().IntVarP(&benchmarkTimeout, "timeout", "t", 0, "seconds to wait before timing out")
	benchmark.Flags().IntVarP(&benchmarkSignersFanoutNumber, "fanout", "f", 1, "how many signers to fanout to on sending a transaction")
	benchmark.Flags().IntVarP(&benchmarkStartDelay, "start-delay", "d", 0, "how many seconds to wait before kicking off the benchmark; useful if network needs to stablize first")
	benchmark.Flags().StringVarP(&benchmarkStrategy, "strategy", "s", "", "whether to use tps, or concurrent load: 'tps' sends 'concurrency' # every second for # of 'iterations'. 'load' sends simultaneous transactions up to 'concurrency' #, until # of 'iterations' is reached")
	benchmark.Flags().BoolVar(&enableJaegerTracing, "jaeger-tracing", false, "enable jaeger tracing")
	benchmark.Flags().BoolVar(&enableElasticTracing, "elastic-tracing", false, "enable elastic tracing")
	benchmark.Flags().StringVar(&tracingTagName, "tracing-name", "", "adds tag to tracing for lookup")
}
