package cmd

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	protocol "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-protocol"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossip2"
	"github.com/quorumcontrol/qc3/gossip2client"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/storage"
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

var activeCounter = 0

func sendTransaction(client *gossip2client.GossipClient) {
	blsKey, _ := bls.NewSignKey()
	treeKey, _ := crypto.ToECDSA(blsKey.Bytes())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": string(randBytes(rand.Intn(400) + 100)),
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)

	if err != nil {
		panic("could not create test tree")
	}

	blockWithHeaders, _ := consensus.SignBlock(unsignedBlock, treeKey)
	testTree.ProcessBlock(blockWithHeaders)

	cborNodes, _ := emptyTree.Nodes()
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      &emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}
	sw := safewrap.SafeWrap{}

	trans := gossip2.Transaction{
		PreviousTip: emptyTip.Bytes(),
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(req).RawData(),
		ObjectID:    []byte(treeDID),
	}

	startTime := time.Now()
	used := map[string]bool{}

	go func(client *gossip2client.GossipClient, did string, startTime time.Time) {
		targetPublicKeyHex := bootstrapPublicKeys[rand.Intn(len(bootstrapPublicKeys))].EcdsaHexPublicKey
		targetPublicKeyBytes, err := hexutil.Decode(targetPublicKeyHex)
		if err != nil {
			panic("can't decode public key")
		}
		targetPublicKey := crypto.ToECDSAPub(targetPublicKeyBytes)
		_, err = client.Subscribe(targetPublicKey, did, 30*time.Second)
		if err != nil {
			panic(fmt.Sprintf("subscription failed %v", err))
		}

		elapsed := time.Since(startTime)
		duration := int(elapsed / time.Millisecond)
		results.Durations = append(results.Durations, duration)
		results.Successes = results.Successes + 1
		activeCounter--
	}(client, treeDID, startTime)

	for len(used) < benchmarkSignersFanoutNumber {
		targetPublicKeyHex := bootstrapPublicKeys[rand.Intn(len(bootstrapPublicKeys))].EcdsaHexPublicKey
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

		err = client.Send(targetPublicKey, protocol.ID(gossip2.NewTransactionProtocol), transBytes, 30*time.Second)
		if err != nil {
			panic(fmt.Sprintf("Couldn't add transaction, %v", err))
		}
	}

	activeCounter++
}

func performTpsBenchmark(client *gossip2client.GossipClient) {
	for benchmarkIterations > 0 {
		for i2 := 1; i2 <= benchmarkConcurrency; i2++ {
			go sendTransaction(client)
		}
		benchmarkIterations--
		time.Sleep(1 * time.Second)
	}
}

func performLoadBenchmark(client *gossip2client.GossipClient) {
	for benchmarkIterations > 0 {
		if activeCounter < benchmarkConcurrency {
			sendTransaction(client)
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
		groupNodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
		group := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", groupNodeStore)
		if group.IsGenesis() {
			testNetMembers := bootstrapMembers(bootstrapPublicKeys)
			group.CreateGenesisState(group.RoundAt(time.Now()), testNetMembers...)
		}
		client := gossip2client.NewGossipClient(group, network.BootstrapNodes())

		time.Sleep(5 * time.Second)

		results = ResultSet{}

		doneCh := make(chan bool, 1)
		defer close(doneCh)

		switch benchmarkStrategy {
		case "tps":
			go performTpsBenchmark(client)
		case "load":
			go performLoadBenchmark(client)
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
