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

	"github.com/ethereum/go-ethereum/log"
	ipfslogging "github.com/ipfs/go-log"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/quorumcontrol/tupelo/gossip3"
	"github.com/shibukawa/configdir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	bootstrapKeyFile = "bootstrap-keys.json"
)

var (
	cfgFile          string
	logLvlName       string
	overrideKeysFile string
	remoteNetwork    bool

	configNamespace string

	remoteConfigName = "remote-network"
)

var logLevels = map[string]log.Lvl{
	"critical": log.LvlCrit,
	"error":    log.LvlError,
	"warn":     log.LvlWarn,
	"info":     log.LvlInfo,
	"debug":    log.LvlDebug,
	"trace":    log.LvlTrace,
}

var zapLogLevels = map[string]string{
	"critical": "panic",
	"error":    "error",
	"warn":     "warn",
	"info":     "info",
	"debug":    "debug",
	"trace":    "debug",
}

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
		if localNetworkNodeCount > 0 && remoteNetwork {
			panic("cannot supply both --local-network N (greater than 0) and --remote-network; please use one or the other")
		}

		logLevel, err := getLogLevel(logLvlName)
		if err != nil {
			panic(err.Error())
		}
		log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(logLevel), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
		err = ipfslogging.SetLogLevel("*", strings.ToUpper(logLvlName))
		if err != nil {
			fmt.Println("unknown ipfs log level")
		}

		if err = gossip3.SetLogLevel(zapLogLevels[logLvlName]); err != nil {
			panic(err)
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

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&logLvlName, "log-level", "L", "error", "Log level")
	rootCmd.PersistentFlags().IntVarP(&localNetworkNodeCount, "local-network", "l", 3, "Run local network with randomly generated keys, specifying number of nodes as argument.")
	rootCmd.PersistentFlags().BoolVarP(&remoteNetwork, "remote-network", "r", false, "Connect to a remote network. Mutually exclusive with -l / --local-network.")
	rootCmd.PersistentFlags().StringVarP(&overrideKeysFile, "override-keys", "k", "", "Path to notary group bootstrap keys file.")
	rootCmd.PersistentFlags().StringVar(&configNamespace, "namespace", "default", "a global config namespace (useful for tests). All configs will be separated using this")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".tupelocmd" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".tupelo")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
