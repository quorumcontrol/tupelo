package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ipfs/go-bitswap"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/client"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/client/pubsubinterfaces/pubsubwrapper"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo/gossip/benchmark"
	"github.com/quorumcontrol/tupelo/nodebuilder"
	"github.com/spf13/cobra"
)

var benchmarkConcurrency int
var benchmarkDuration int
var benchmarkStartDelay int
var benchmarkTimeout int

// benchmark represents the shell command
var benchmarkCmd = &cobra.Command{
	Use:    "benchmark",
	Short:  "runs a set of operations against a network at specified concurrency and duration",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		config := nodebuilderConfig
		if config == nil {
			panic(fmt.Errorf("you must specify a config"))
		}
		nb := &nodebuilder.NodeBuilder{Config: config}
		nb.StartTracing()

		p2pHost, peer, err := p2p.NewHostAndBitSwapPeer(
			ctx,
			p2p.WithDiscoveryNamespaces(nb.Config.NotaryGroupConfig.ID),
			p2p.WithBitswapOptions(bitswap.ProvideEnabled(false)),
		)
		if err != nil {
			panic(fmt.Errorf("error creating host: %w", err))
		}

		if benchmarkStartDelay > 0 {
			time.Sleep(time.Duration(benchmarkStartDelay) * time.Second)
		}

		_, err = p2pHost.Bootstrap(config.BootstrapNodes)
		if err != nil {
			panic(fmt.Errorf("error bootstrapping: %w", err))
		}

		group, err := nb.NotaryGroup()
		if err != nil {
			panic(fmt.Errorf("error getting notary group: %v", err))
		}

		if err = p2pHost.WaitForBootstrap(1+len(group.Signers)/2, 15*time.Second); err != nil {
			panic(err)
		}

		cli := client.New(group, pubsubwrapper.WrapLibp2p(p2pHost.GetPubSub()), peer)
		err = cli.Start(ctx)
		if err != nil {
			panic(fmt.Errorf("error starting client: %v", err))
		}

		b := benchmark.NewBenchmark(cli, benchmarkConcurrency, time.Duration(benchmarkDuration)*time.Second, time.Duration(benchmarkTimeout)*time.Second)

		results := b.Run(ctx)

		out, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(out))
	},
}

func init() {
	rootCmd.AddCommand(benchmarkCmd)
	benchmarkCmd.Flags().IntVarP(&benchmarkConcurrency, "concurrency", "c", 1, "how many transactions to execute at once")
	benchmarkCmd.Flags().IntVarP(&benchmarkDuration, "duration", "d", 10, "how many seconds to run benchmark for")
	benchmarkCmd.Flags().IntVarP(&benchmarkTimeout, "timeout", "t", 10, "how many seconds to timeout waiting for Txs to complete")
	benchmarkCmd.Flags().IntVar(&benchmarkStartDelay, "delay", 0, "how many seconds to wait before kicking off the benchmark; useful if network needs to stabilize first")
}
