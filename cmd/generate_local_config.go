package cmd

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/messages/v2/build/go/config"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo-go-sdk/signatures"
	"github.com/quorumcontrol/tupelo/nodebuilder"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/spf13/cobra"
)

var localConfigNodeCount int
var localConfigPath string

const perfTestGroupName = "perfgroup"

type fakeTest struct {
	testing.TB
}

func blsKeys(size int) []*bls.SignKey {
	keys := make([]*bls.SignKey, size)
	for i := 0; i < size; i++ {
		keys[i] = bls.MustNewSignKey()
	}
	return keys
}

// TODO: this is duplicative with testnotarygroup.NewTestSet but that requires an
// instance of testing.TB
func newKeySet(size int) (*testnotarygroup.TestSet, error) {
	signKeys := blsKeys(size)
	verKeys := make([]*bls.VerKey, len(signKeys))
	pubKeys := make([]*ecdsa.PublicKey, len(signKeys))
	ecdsaKeys := make([]*ecdsa.PrivateKey, len(signKeys))
	signKeysByAddress := make(map[string]*bls.SignKey)
	for i, signKey := range signKeys {
		ecdsaKey, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		verKeys[i] = signKey.MustVerKey()
		pubKeys[i] = &ecdsaKey.PublicKey
		ecdsaKeys[i] = ecdsaKey
		addr, err := signatures.Address(signatures.BLSToOwnership(verKeys[i]))
		if err != nil {
			return nil, err
		}
		signKeysByAddress[addr.String()] = signKey

	}

	return &testnotarygroup.TestSet{
		SignKeys:          signKeys,
		VerKeys:           verKeys,
		PubKeys:           pubKeys,
		EcdsaKeys:         ecdsaKeys,
		SignKeysByAddress: signKeysByAddress,
	}, nil
}

func writeLocalConfigs() error {
	keys, err := newKeySet(localConfigNodeCount)
	if err != nil {
		return err
	}

	for i := 0; i < localConfigNodeCount; i++ {
		hc := &nodebuilder.HumanConfig{
			NotaryGroupConfig: "../" + perfTestGroupName + ".toml",
			PrivateKeySet: &nodebuilder.HumanPrivateKeySet{
				SignKeyHex: hexutil.Encode(keys.SignKeys[i].Bytes()),
				DestKeyHex: hexutil.Encode(crypto.FromECDSA(keys.EcdsaKeys[i])),
			},
		}
		nodeConfigPath := path.Join(localConfigPath, fmt.Sprintf("node-%d", i))
		if err := os.MkdirAll(nodeConfigPath, 0755); err != nil {
			return err
		}

		f, err := os.Create(path.Join(nodeConfigPath, "config.toml"))
		if err != nil {
			return err
		}
		defer f.Close()

		encoder := toml.NewEncoder(f)
		err = encoder.Encode(hc)
		if err != nil {
			return err
		}
	}
	// now write out the notary group
	ngConfig := types.TomlConfig{
		NotaryGroup: &config.NotaryGroup{
			Id: perfTestGroupName,
			BootstrapAddresses: []string{
				// TODO: hard coded the bootstrap addr for now
				"/ip4/172.16.238.10/tcp/34001/ipfs/16Uiu2HAm3TGSEKEjagcCojSJeaT5rypaeJMKejijvYSnAjviWwV5",
			},
		},
	}
	for i := 0; i < localConfigNodeCount; i++ {
		ngConfig.Signers = append(ngConfig.Signers, types.TomlPublicKeySet{
			VerKeyHex:  hexutil.Encode(keys.VerKeys[i].Bytes()),
			DestKeyHex: hexutil.Encode(crypto.FromECDSAPub(keys.PubKeys[i])),
		})
	}

	f, err := os.Create(path.Join(localConfigPath, perfTestGroupName+".toml"))
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	err = encoder.Encode(ngConfig)
	if err != nil {
		return err
	}

	return nil
}

type serviceCompose struct {
	DependsOn []string `yaml:"depends_on"`
	Build     string   `yaml:"build"`
	Volumes   []string `yaml:"volumes"`
	Command   []string `yaml:"command"`
}

type composeFile map[string]*serviceCompose

func writeDockerCompose() error {
	composeSnippet := make(composeFile)
	// write the netdelay depends_on snippet

	depends := make([]string, localConfigNodeCount)
	for i := 0; i < localConfigNodeCount; i++ {
		depends[i] = "node" + strconv.Itoa(i)
	}

	composeSnippet["netdelay"] = &serviceCompose{
		DependsOn: depends,
	}

	for i := 0; i < localConfigNodeCount; i++ {
		composeSnippet["node"+strconv.Itoa(i)] = &serviceCompose{
			DependsOn: []string{"bootstrap"},
			Build:     ".",
			Volumes: []string{
				"./configs/localperftest:/configs",
			},
			Command: []string{
				"node", "-L", "${TUPELO_LOG_LEVEL:-warn}", "--config", "/configs/node-" + strconv.Itoa(i) + "/config.toml",
			},
		}
	}

	data, err := yaml.Marshal(composeSnippet)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(path.Join(localConfigPath, "compose-snippet.yml"), data, 0644)
	if err != nil {
		return err
	}
	return nil
}

// generateNodeKeysCmd represents the generateNodeKeys command
var generateLocalConfigs = &cobra.Command{
	Use:   "generate-local-config",
	Short: "Generate a new set configs suitable for running an n-sized notary group",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {

		// and now we make the notary group config
		err := writeLocalConfigs()
		if err != nil {
			panic(err)
		}
		err = writeDockerCompose()
		if err != nil {
			panic(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(generateLocalConfigs)
	generateLocalConfigs.Flags().IntVar(&localConfigNodeCount, "count", 3, "how many config sets to generate")
	generateLocalConfigs.Flags().StringVar(&localConfigPath, "path", "./configs/localperftest", "the directory to store the files")
}
