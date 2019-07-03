// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo/nodebuilder"

	"github.com/spf13/cobra"
)

var (
	testnodePort int

	enableJaegerTracing  bool
	enableElasticTracing bool
)

// testnodeCmd represents the test-node command
var testnodeCmd = &cobra.Command{
	Use:   "test-node [index of key]",
	Short: "Run a tupelo node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		remote.Start()

		config := nodebuilderConfig
		if config == nil {
			var err error
			config, err = nodebuilder.LegacyConfig(configNamespace, testnodePort, enableElasticTracing, enableJaegerTracing, overrideKeysFile)
			if err != nil {
				panic(fmt.Errorf("error getting legacy config: %v", err))
			}
		}

		nb := &nodebuilder.NodeBuilder{Config: config}
		err := nb.Start(ctx)
		if err != nil {
			panic(fmt.Errorf("error starting: %v", err))
		}
		err = nb.Host().WaitForBootstrap(len(config.NotaryGroupConfig.Signers)/2, 30*time.Second)
		if err != nil {
			panic(fmt.Errorf("error starting: %v", err))
		}
		fmt.Printf("started signer host %s on %v\n", nb.Host().Identity(), nb.Host().Addresses())
		select {}
	},
}

func init() {
	rootCmd.AddCommand(testnodeCmd)
	testnodeCmd.Flags().IntVarP(&testnodePort, "port", "p", 0, "what port will the node listen on")
	testnodeCmd.Flags().BoolVar(&enableJaegerTracing, "jaeger-tracing", false, "enable jaeger tracing")
	testnodeCmd.Flags().BoolVar(&enableElasticTracing, "elastic-tracing", false, "enable elastic tracing")
}
