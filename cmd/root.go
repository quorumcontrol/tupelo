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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/log"
	ipfslogging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/shibukawa/configdir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	bootstrapKeyFile = "bootstrap-keys.json"
)

var (
	bootstrapPublicKeys []*PublicKeySet
	cfgFile             string
	logLvlName          string
	newKeysFile         string
	overrideKeysFile    string

	localConfig  = configDir("local-network")
	remoteConfig = configDir("remote-network")
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

func configDir(namespace string) string {
	conf := configdir.New("tupelo", namespace)
	folders := conf.QueryFolders(configdir.Global)

	return folders[0].Path
}

func readConfig(path string, filename string) ([]byte, error) {
	_, err := os.Stat(filepath.Join(path, filename))
	if os.IsNotExist(err) {
		return nil, nil
	}

	return ioutil.ReadFile(filepath.Join(path, filename))
}

func writeFile(parentDir string, filename string, data []byte) error {
	err := os.MkdirAll(parentDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	return ioutil.WriteFile(filepath.Join(parentDir, filename), data, 0644)
}

func loadBootstrapKeyFile(path string) ([]*PublicKeySet, error) {
	var keySet []*PublicKeySet

	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading path %v: %v", path, err)
	}

	err = unmarshalKeys(keySet, file)

	return keySet, err
}

func saveBootstrapKeys(keys []*PublicKeySet) error {
	bootstrapKeyJson, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("Error marshaling bootstrap keys: %v", err)
	}

	err = writeFile(remoteConfig, bootstrapKeyFile, bootstrapKeyJson)
	if err != nil {
		return fmt.Errorf("error writing bootstrap keys: %v", err)
	}

	return nil
}

func readBootstrapKeys() ([]*PublicKeySet, error) {
	var keySet []*PublicKeySet
	err := loadKeyFile(keySet, remoteConfig, bootstrapKeyFile)

	return keySet, err
}

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
		logLevel, err := getLogLevel(logLvlName)
		if err != nil {
			panic(err.Error())
		}
		log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(logLevel), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
		err = ipfslogging.SetLogLevel("*", strings.ToUpper(logLvlName))
		if err != nil {
			fmt.Println("unknown ipfs log level")
		}

		if overrideKeysFile != "" {
			publicKeys, err := loadPublicKeyFile(overrideKeysFile)
			if err != nil {
				panic(fmt.Sprintf("Error loading public keys: %v", err))
			}

			bootstrapPublicKeys = publicKeys
		} else {
			publicKeys, err := readBootstrapKeys()
			if err != nil {
				fmt.Printf("error loading stored bootstrap keys: %v", err)
			} else {
				bootstrapPublicKeys = publicKeys
			}
		}
		log.Info("loaded public keys", "count", len(bootstrapPublicKeys))
	},
	Run: func(cmd *cobra.Command, args []string) {
		if newKeysFile != "" {
			keys, err := loadBootstrapKeyFile(newKeysFile)
			if err != nil {
				panic(fmt.Sprintf("Error loading bootstrap keys: %v", err))
			}

			err = saveBootstrapKeys(keys)
			if err != nil {
				panic(fmt.Sprintf("Error saving bootstrap keys: %v", err))
			}
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
	rootCmd.PersistentFlags().StringVarP(&overrideKeysFile, "override-keys", "k", "", "path to notary group bootstrap keys file")
	rootCmd.Flags().StringVarP(&newKeysFile, "import-boot-keys", "i", "", "Path of a notary group key file to import")
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
