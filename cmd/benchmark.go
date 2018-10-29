package cmd

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossipclient"
	"github.com/quorumcontrol/storage"
	"github.com/spf13/cobra"
)

var wg sync.WaitGroup

type ResultSet struct {
	Successes       int
	Failures        int
	Durations       []int
	AverageDuration int
}

var results ResultSet
var benchmarkConcurrency int
var benchmarkIterations int

func runBenchmark(iterations int, client *gossipclient.GossipClient) {
	defer wg.Done()

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	for i := 1; i <= iterations; i++ {
		key, err := crypto.GenerateKey()
		if err != nil {
			panic("could not generate key")
		}

		chain, err := consensus.NewSignedChainTree(key.PublicKey, nodeStore)
		if err != nil {
			panic(fmt.Sprintf("error generating chain: %v", err))
		}

		var remoteTip string
		if !chain.IsGenesis() {
			remoteTip = chain.Tip().String()
		}

		chainID, _ := chain.Id()
		log.Debug("going to send new transaction", "chain", chainID)

		startTime := time.Now()

		channel := make(chan string, 1)
		go func() {
			resp, _ := client.PlayTransactions(chain, key, remoteTip, []*chaintree.Transaction{
				{
					Type: consensus.TransactionTypeSetData,
					Payload: consensus.SetDataPayload{
						Path:  "this/is/a/test/path",
						Value: "somevalue",
					},
				},
			})
			channel <- resp.Tip.String()
		}()

		select {
		case res := <-channel:
			elapsed := time.Since(startTime)
			duration := int(elapsed / time.Millisecond)
			// TODO thread safety
			results.Durations = append(results.Durations, duration)
			results.Successes = results.Successes + 1
			log.Debug("new tip confirmed", "chain", chainID, "tip", res, "duration", duration)
		case <-time.After(60 * time.Second):
			results.Failures = results.Failures + 1
			log.Debug("transaction timed out", "chain", chainID)
		}
	}
}

// benchmark represents the shell command
var benchmark = &cobra.Command{
	Use:    "benchmark",
	Short:  "runs a set of operations against a network at specified concurrency",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		groupNodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

		group := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", groupNodeStore)
		if group.IsGenesis() {
			testNetMembers := bootstrapMembers(bootstrapPublicKeys)
			group.CreateGenesisState(group.RoundAt(time.Now()), testNetMembers...)
		}

		client := gossipclient.NewGossipClient(group)
		client.Start()

		// Prime it
		wg.Add(1)
		runBenchmark(2, client)
		time.Sleep(10 * time.Second)
		results = ResultSet{}

		iterationPerThread := benchmarkIterations / benchmarkConcurrency

		for i := 1; i <= benchmarkConcurrency; i++ {
			wg.Add(1)
			go runBenchmark(iterationPerThread, client)
		}
		wg.Wait()

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
}
