package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/gossip2"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/spf13/cobra"
)

type ResultSet struct {
	Successes       int
	Failures        int
	Durations       []int
	AverageDuration int
}

var results ResultSet
var benchmarkConcurrency int
var benchmarkIterations int
var benchmarkTimeout int
var benchmarkStrategy string
var benchmarkSignersFanoutNumber int

var conflictSets sync.Map
var activeCounter = 0

func sendTransaction(host *p2p.Host) {
	startTime := time.Now()
	trans := gossip2.Transaction{
		ObjectID:    randBytes(32),
		PreviousTip: []byte(""),
		NewTip:      randBytes(49),
		Payload:     randBytes(rand.Intn(400) + 100),
	}

	used := map[string]bool{}

	for len(used) < benchmarkSignersFanoutNumber {
		targetPublicKeyHex := bootstrapPublicKeys[rand.Intn(len(bootstrapPublicKeys)-1)].EcdsaHexPublicKey
		if used[targetPublicKeyHex] {
			continue
		}
		used[targetPublicKeyHex] = true

		targetPublicKeyBytes, err := hexutil.Decode(targetPublicKeyHex)
		if err != nil {
			panic("can't decode public key")
		}
		targetPublicKey := crypto.ToECDSAPub(targetPublicKeyBytes)

		transBytes, err := trans.MarshalMsg(nil)
		if err != nil {
			panic("can't encode transaction")
		}

		err = host.Send(targetPublicKey, gossip2.NewTransactionProtocol, transBytes)
		if err != nil {
			panic(fmt.Sprintf("Couldn't add transaction, %v", err))
		}
	}

	conflictSets.Store(string(trans.ToConflictSet().ID()), startTime)
	activeCounter++
}

func pollForResults(host *p2p.Host) {
	for _ = range time.Tick(300 * time.Millisecond) {
		targetPublicKeyHex := bootstrapPublicKeys[rand.Intn(len(bootstrapPublicKeys)-1)].EcdsaHexPublicKey
		targetPublicKeyBytes, err := hexutil.Decode(targetPublicKeyHex)
		if err != nil {
			panic("can't decode public key")
		}
		targetPublicKey := crypto.ToECDSAPub(targetPublicKeyBytes)

		conflictSets.Range(func(key, val interface{}) bool {
			csid := key.(string)
			startTime := val.(time.Time)

			csq := gossip2.ConflictSetQuery{Key: []byte(csid)}
			csqBytes, err := csq.MarshalMsg(nil)
			if err != nil {
				panic("Can't marshal csq")
			}
			responseBytes, err := host.SendAndReceive(targetPublicKey, gossip2.IsDoneProtocol, csqBytes)
			if err != nil {
				fmt.Printf("Error on send/receive of conflict set query: %v\n", err)
				return true
			}

			conflictSetResponse := gossip2.ConflictSetQueryResponse{}
			_, err = conflictSetResponse.UnmarshalMsg(responseBytes)
			if err != nil {
				panic("Can't unmarshal csqr")
			}

			if conflictSetResponse.Done {
				elapsed := time.Since(startTime)
				duration := int(elapsed / time.Millisecond)
				results.Durations = append(results.Durations, duration)
				results.Successes = results.Successes + 1
				conflictSets.Delete(csid)
				activeCounter--
			}
			return true
		})
	}
}

func performTpsBenchmark(host *p2p.Host, done chan bool) {
	for benchmarkIterations > 0 {
		for i2 := 1; i2 <= benchmarkConcurrency; i2++ {
			go sendTransaction(host)
		}
		benchmarkIterations--
		time.Sleep(1 * time.Second)

		// startTime := time.Now()
		// for i2 := 1; i2 <= benchmarkConcurrency; i2++ {
		// 	sendTransaction(host)
		// }
		// diff := (1 * time.Second) - time.Since(startTime)

		// if diff < 0 {
		// 	fmt.Printf("ERROR, sending batch of transactions took longer than one second: %d", diff)
		// 	done <- true
		// 	break
		// }

		// time.Sleep(diff)
		// benchmarkIterations--
	}
}

func performLoadBenchmark(host *p2p.Host) {
	for benchmarkIterations > 0 {
		if activeCounter < benchmarkConcurrency {
			sendTransaction(host)
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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		key, err := crypto.GenerateKey()
		if err != nil {
			panic("error generating key")
		}
		host, err := p2p.NewHost(ctx, key, p2p.GetRandomUnusedPort())
		host.Bootstrap(network.BootstrapNodes())

		time.Sleep(5 * time.Second)

		results = ResultSet{}

		doneCh := make(chan bool, 1)
		defer close(doneCh)

		go pollForResults(host)

		switch benchmarkStrategy {
		case "tps":
			go performTpsBenchmark(host, doneCh)
		case "load":
			go performLoadBenchmark(host)
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
				fmt.Println("WARNING: timeout was triggered")
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
		}

		out, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(out))
	},
}

func init() {
	rootCmd.AddCommand(benchmark)
	benchmark.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")
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
