package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/quorumcontrol/tupelo/signer/nodebuilder"

	"github.com/spf13/cobra"
)

func loadBootstrapKeyFile(path string) ([]*nodebuilder.LegacyPublicKeySet, error) {
	var keySet []*nodebuilder.LegacyPublicKeySet

	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading path %v: %v", path, err)
	}

	err = unmarshalKeys(keySet, file)

	return keySet, err
}

func unmarshalKeys(keySet interface{}, bytes []byte) error {
	if bytes != nil {
		err := json.Unmarshal(bytes, keySet)
		if err != nil {
			return err
		}
	}

	return nil
}

func saveBootstrapKeys(keys []*nodebuilder.LegacyPublicKeySet) error {
	bootstrapKeyJson, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("Error marshaling bootstrap keys: %v", err)
	}

	err = writeFile(configDir(remoteConfigName), bootstrapKeyFile, bootstrapKeyJson)
	if err != nil {
		return fmt.Errorf("error writing bootstrap keys: %v", err)
	}

	return nil
}

var importKeysCmd = &cobra.Command{
	Use:   "import-keys /path/to/key",
	Short: "import a key file",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		keys, err := loadBootstrapKeyFile(args[0])
		if err != nil {
			panic(fmt.Sprintf("Error loading bootstrap keys: %v", err))
		}

		err = saveBootstrapKeys(keys)
		if err != nil {
			panic(fmt.Sprintf("Error saving bootstrap keys: %v", err))
		}
	},
}

func init() {
	rootCmd.AddCommand(importKeysCmd)
}
