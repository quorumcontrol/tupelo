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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/quorumcontrol/tupelo/nodebuilder"

	"github.com/ethereum/go-ethereum/log"
	ipfslogging "github.com/ipfs/go-log"
	"github.com/shibukawa/configdir"
	"github.com/spf13/cobra"
)

const (
	bootstrapKeyFile      = "bootstrap-keys.json"
	defaultLocalNodeCount = 3
)

var (
	configFilePath    string
	nodebuilderConfig *nodebuilder.Config

	logLvlName       string
	overrideKeysFile string
	remoteNetwork    bool

	configNamespace string

	remoteConfigName      = "remote-network"
	localNetworkNodeCount int
)

var logLevels = map[string]log.Lvl{
	"critical": log.LvlCrit,
	"error":    log.LvlError,
	"warn":     log.LvlWarn,
	"info":     log.LvlInfo,
	"debug":    log.LvlDebug,
	"trace":    log.LvlTrace,
}

//TODO: restore this in gossip4
// var zapLogLevels = map[string]string{
//	"critical": "panic",
//	"error":    "error",
//	"warn":     "warn",
//	"info":     "info",
//	"debug":    "debug",
//	"trace":    "debug",
// }

func getLogLevel(lvlName string) (log.Lvl, error) {
	lvl, ok := logLevels[lvlName]
	if !ok {
		return -1, fmt.Errorf("Invalid log level %v. Must be either `critical`, `error`, `warn`, `info`, `debug`, or `trace`.", lvlName)
	}

	return lvl, nil
}

func configDir(namespace string) string {
	conf := configdir.New("tupelo", filepath.Join(configNamespace, namespace))
	folders := conf.QueryFolders(configdir.Global)
	if err := os.MkdirAll(folders[0].Path, 0700); err != nil {
		panic(err)
	}
	return folders[0].Path
}

func writeFile(parentDir string, filename string, data []byte) error {
	err := os.MkdirAll(parentDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	return ioutil.WriteFile(filepath.Join(parentDir, filename), data, 0644)
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tupelo",
	Short: "Tupelo interface",
	Long:  `Tupelo is a distributed ledger optimized for ownership`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logLevel, err := getLogLevel(logLvlName)
		if err != nil {
			panic(err.Error())
		}
		log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(logLevel), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
		if err := ipfslogging.SetLogLevel("*", strings.ToUpper(logLvlName)); err != nil {
			fmt.Println("unknown ipfs log level")
		}

		// TODO: Restore this in gossip4
		// if err := gossip3.SetLogLevel(zapLogLevels[logLvlName]); err != nil {
		//	panic(err)
		// }

		if remoteNetwork {
			if localNetworkNodeCount > 0 {
				panic("cannot supply both --local-network N (greater than 0) and --remote-network; please use one or the other")
			}
		} else if overrideKeysFile != "" {
			if localNetworkNodeCount > 0 {
				panic("cannot supply both --local-network N (greater than 0) and --override-keys; " +
					"please use one or the other")
			}

			remoteNetwork = true
		} else {
			if localNetworkNodeCount < 0 {
				localNetworkNodeCount = defaultLocalNodeCount
			}
		}

		if configFilePath != "" {
			config, err := loadTomlConfig(configFilePath)
			if err != nil {
				panic(err)
			}

			nodebuilderConfig = config
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func loadTomlConfig(path string) (*nodebuilder.Config, error) {
	c, err := nodebuilder.TomlToConfig(path)
	if err != nil {
		return nil, fmt.Errorf("error getting config from %s: %v", path, err)
	}
	return c, nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFilePath, "config", "", "path to a toml config file")
	rootCmd.PersistentFlags().StringVarP(&logLvlName, "log-level", "L", "error", "Log level")
	rootCmd.PersistentFlags().IntVarP(&localNetworkNodeCount, "local-network", "l", -1, fmt.Sprintf(
		"Run local network with randomly generated keys, specifying number of nodes as argument (default: %d).", defaultLocalNodeCount))
	rootCmd.PersistentFlags().BoolVarP(&remoteNetwork, "remote-network", "r", false, "Connect to a remote network. Mutually exclusive with -l / --local-network.")
	rootCmd.PersistentFlags().StringVarP(&overrideKeysFile, "override-keys", "k", "", "Path to notary group bootstrap keys file.")
	rootCmd.PersistentFlags().StringVar(&configNamespace, "namespace", "default", "a global config namespace (useful for tests). All configs will be separated using this")
}
