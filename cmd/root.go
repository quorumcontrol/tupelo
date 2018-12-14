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
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/log"
	ipfslogging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var logLevels = map[string]log.Lvl{
	"critical": log.LvlCrit,
	"error":    log.LvlError,
	"warn":     log.LvlWarn,
	"info":     log.LvlInfo,
	"debug":    log.LvlDebug,
	"trace":    log.LvlTrace,
}

func getLogLevel(lvlName string) (log.Lvl, error) {
	lvl, ok := logLevels[lvlName]
	if !ok {
		return -1, fmt.Errorf("Invalid log level %v. Must be either `critical`, `error`, `warn`, `info`, `debug`, or `trace`.", lvlName)
	}

	return lvl, nil
}

var logLvlName string
var cfgFile string
var bootstrapPublicKeysFile string
var bootstrapPublicKeys []*PublicKeySet

type PublicKeySet struct {
	BlsHexPublicKey   string `json:"blsHexPublicKey,omitempty"`
	EcdsaHexPublicKey string `json:"ecdsaHexPublicKey,omitempty"`
	PeerIDBase58Key   string `json:"peerIDBase58Key,omitempty"`
}

type PrivateKeySet struct {
	BlsHexPrivateKey   string `json:"blsHexPrivateKey,omitempty"`
	EcdsaHexPrivateKey string `json:"ecdsaHexPrivateKey,omitempty"`
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tupelo",
	Short: "Tupelo interface",
	Long:  `Tupelo is a distributed ledger optimized for ownership`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if bootstrapPublicKeysFile != "" {
			var err error
			bootstrapPublicKeys, err = loadPublicKeyFile(bootstrapPublicKeysFile)
			if err != nil {
				panic(fmt.Sprintf("Error loading public keys: %v", err))
			}
		}

		logLevel, err := getLogLevel(logLvlName)
		if err != nil {
			panic(err.Error())
		}
		log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(logLevel), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
		err = ipfslogging.SetLogLevel("*", strings.ToUpper(logLvlName))
		if err != nil {
			fmt.Println("unkown ipfs log level")
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

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tupelo.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.PersistentFlags().StringVarP(&logLvlName, "log-level", "L", "error", "Log level")
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
