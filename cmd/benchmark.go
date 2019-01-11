package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	gossip3client "github.com/quorumcontrol/tupelo/gossip3/client"
	gossip3remote "github.com/quorumcontrol/tupelo/gossip3/remote"
	gossip3testhelpers "github.com/quorumcontrol/tupelo/gossip3/testhelpers"
	gossip3types "github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
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

var activeCounter = 0

func measureTransaction(client *gossip3client.Client, group *gossip3types.NotaryGroup, did string) {

	startTime := time.Now()

	respChan, err := client.Subscribe(group.GetRandomSigner(), did, 30*time.Second)
	if err != nil {
		results.Errors = append(results.Errors, fmt.Errorf("subscription failed %v", err).Error())
		results.Failures = results.Failures + 1
		activeCounter--
		return
	}

	// Wait for response
	resp := <-respChan

	if resp == nil {
		results.Errors = append(results.Errors, did)
		results.Failures = results.Failures + 1
		activeCounter--
		return
	}

	elapsed := time.Since(startTime)
	duration := int(elapsed / time.Millisecond)
	results.Durations = append(results.Durations, duration)
	results.Successes = results.Successes + 1
	activeCounter--
}

func sendTransaction(client *gossip3client.Client, group *gossip3types.NotaryGroup, shouldMeasure bool) {
	fakeT := &testing.T{}
	trans := gossip3testhelpers.NewValidTransaction(fakeT)

	used := map[string]bool{}

	if shouldMeasure {
		go measureTransaction(client, group, string(trans.ObjectID))
	}

	tries := 0

	for len(used) < benchmarkSignersFanoutNumber {
		target := group.GetRandomSigner()
		used[target.ID] = true

		err := client.SendTransaction(target, &trans)
		if err != nil {
			tries++
			used[target.ID] = false
			if tries > 5 {
				panic(fmt.Sprintf("Couldn't add transaction, %v", err))
			}
		}
	}

	activeCounter++
}

func performTpsBenchmark(client *gossip3client.Client, group *gossip3types.NotaryGroup) {
	for benchmarkIterations > 0 {
		for i2 := 1; i2 <= benchmarkConcurrency; i2++ {
			go sendTransaction(client, group, true)
		}
		benchmarkIterations--
		time.Sleep(1 * time.Second)
	}
}

func performTpsNoAckBenchmark(client *gossip3client.Client, group *gossip3types.NotaryGroup) {
	for benchmarkIterations > 0 {
		for i2 := 1; i2 <= benchmarkConcurrency; i2++ {
			// measure only the final iteration
			if benchmarkIterations <= 1 {
				go sendTransaction(client, group, true)
			} else {
				go sendTransaction(client, group, false)
			}
		}
		benchmarkIterations--
		time.Sleep(1 * time.Second)
	}
}

func performLoadBenchmark(client *gossip3client.Client, group *gossip3types.NotaryGroup) {
	for benchmarkIterations > 0 {
		if activeCounter < benchmarkConcurrency {
			sendTransaction(client, group, true)
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

		key, err := crypto.GenerateKey()
		if err != nil {
			panic(fmt.Sprintf("error generating key: %v", err))
		}

		p2pHost, err := p2p.NewLibP2PHost(ctx, key, 0)
		if err != nil {
			panic(fmt.Sprintf("error setting up p2p host: %v", err))
		}
		p2pHost.Bootstrap(p2p.BootstrapNodes())
		gossip3remote.NewRouter(p2pHost)

		group := setupNotaryGroup(nil, bootstrapPublicKeys)
		group.SetupAllRemoteActors(&key.PublicKey)

		client := gossip3client.New(group)

		// Give time to bootstrap
		time.Sleep(15 * time.Second)

		results = ResultSet{}

		doneCh := make(chan bool, 1)
		defer close(doneCh)

		switch benchmarkStrategy {
		case "tps":
			go performTpsBenchmark(client, group)
		case "tps-no-ack":
			go performTpsNoAckBenchmark(client, group)
		case "load":
			go performLoadBenchmark(client, group)
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
			select {
			case <-doneCh:
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
	benchmark.Flags().IntVarP(&benchmarkIterations, "iterations", "i", 10, "how many transactions to execute total")
	benchmark.Flags().IntVarP(&benchmarkTimeout, "timeout", "t", 0, "seconds to wait before timing out")
	benchmark.Flags().IntVarP(&benchmarkSignersFanoutNumber, "fanout", "f", 1, "how many signers to fanout to on sending a transaction")
	benchmark.Flags().StringVarP(&benchmarkStrategy, "strategy", "s", "", "whether to use tps, or concurrent load: 'tps' sends 'concurrency' # every second for # of 'iterations'. 'load' sends simultaneous transactions up to 'concurrency' #, until # of 'iterations' is reached")
}

func randBytes(length int) []byte {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("couldn't generate random bytes")
	}
	return b
}
