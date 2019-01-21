package cmd

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/gossip3/actors"

	"github.com/quorumcontrol/tupelo/wallet/walletrpc"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"

	gossip3client "github.com/quorumcontrol/tupelo/gossip3/client"
	gossip3messages "github.com/quorumcontrol/tupelo/gossip3/messages"
	gossip3remote "github.com/quorumcontrol/tupelo/gossip3/remote"
	gossip3types "github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/spf13/cobra"
)

func unmarshalKeys(keySet interface{}, bytes []byte) error {
	if bytes != nil {
		err := json.Unmarshal(bytes, keySet)
		if err != nil {
			return err
		}
	}

	return nil
}

func loadKeyFile(keySet interface{}, path string, name string) error {
	jsonBytes, err := readConfig(path, name)
	if err != nil {
		return fmt.Errorf("error loading key file: %v", err)
	}

	return unmarshalKeys(keySet, jsonBytes)
}

func loadPublicKeyFile(path string) ([]*PublicKeySet, error) {
	var keySet []*PublicKeySet
	var err error

	err = loadKeyFile(&keySet, path, publicKeyFile)

	return keySet, err
}

func loadPrivateKeyFile(path string) ([]*PrivateKeySet, error) {
	var keySet []*PrivateKeySet
	var err error

	err = loadKeyFile(&keySet, path, privateKeyFile)

	return keySet, err
}

func loadLocalKeys(num int) ([]*PrivateKeySet, []*PublicKeySet, error) {
	privateKeys, err := loadPrivateKeyFile(localConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("error loading private keys: %v", err)
	}

	publicKeys, err := loadPublicKeyFile(localConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("error loading public keys: %v", err)
	}

	savedKeyCount := len(privateKeys)
	if savedKeyCount < num {
		extraPrivateKeys, extraPublicKeys, err := generateKeySets(num - savedKeyCount)
		if err != nil {
			return nil, nil, fmt.Errorf("error generating extra node keys: %v", err)
		}

		combinedPrivateKeys := append(privateKeys, extraPrivateKeys...)
		combinedPublicKeys := append(publicKeys, extraPublicKeys...)

		err = writeJSONKeys(combinedPrivateKeys, combinedPublicKeys, localConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("error writing extra node keys: %v", err)
		}

		return combinedPrivateKeys, combinedPublicKeys, nil
	} else if savedKeyCount > num {
		return privateKeys[:num], publicKeys[:num], nil
	} else {
		return privateKeys, publicKeys, nil
	}
}

func syncerActorName(signer *gossip3types.Signer) string {
	return "tupelo-"+signer.ID
}

func signerCommitPath(storagePath string, signer *gossip3types.Signer) (path string) {
	path = filepath.Join(storagePath, signer.ID+"-commit")
	os.MkdirAll(path, 0755)
	return
}

func signerCurrentPath(storagePath string, signer *gossip3types.Signer) (path string) {
	path = filepath.Join(storagePath, signer.ID+"-current")
	os.MkdirAll(path, 0755)
	return
}

func setupLocalSigner(ctx context.Context, group *gossip3types.NotaryGroup, ecdsaKeyHex string, blsKeyHex string, storagePath string) *gossip3types.Signer {
	ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
	if err != nil {
		panic(fmt.Sprintf("error decoding ecdsa key: %v", err))
	}

	blsKey := bls.BytesToSignKey(hexutil.MustDecode(blsKeyHex))

	signer := gossip3types.NewLocalSigner(&ecdsaKey.PublicKey, blsKey)

	commitPath := signerCommitPath(storagePath, signer)
	currentPath := signerCurrentPath(storagePath, signer)

	commitStore, err := storage.NewBadgerStorage(commitPath)
	if err != nil {
		panic(fmt.Sprintf("error setting up badger storage: %v", err))
	}
	currenStore, err := storage.NewBadgerStorage(currentPath)
	if err != nil {
		panic(fmt.Sprintf("error setting up badger storage: %v", err))
	}

	syncer, err := actor.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              signer,
		NotaryGroup:       group,
		CommitStore:       commitStore,
		CurrentStateStore: currenStore,
	}), syncerActorName(signer))
	if err != nil {
		panic(fmt.Sprintf("error spawning actor: %v", err))
	}
	signer.Actor = syncer

	go func() {
		<-ctx.Done()
		syncer.Poison()
	}()

	group.AddSigner(signer)

	return signer
}

func setupLocalNetwork(ctx context.Context, nodeCount int) *gossip3types.NotaryGroup {
	privateKeys, _, err := loadLocalKeys(nodeCount)
	if err != nil {
		panic(fmt.Sprintf("error generating node keys: %v", err))
	}

	group := gossip3types.NewNotaryGroup("local notary group")

	for _, keys := range privateKeys {
		log.Info("setting up gossip node")
		setupLocalSigner(ctx, group, keys.EcdsaHexPrivateKey, keys.BlsHexPrivateKey, localConfig)
	}

	return group
}

func bootstrapAddresses(bootstrapHost p2p.Node) []string {
	addresses := bootstrapHost.Addresses()
	for _, addr := range addresses {
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") {
			return []string{addrStr}
		}
	}
	return nil
}

func panicWithoutTLSOpts() {
	if certFile == "" || keyFile == "" {
		var msg strings.Builder
		if certFile == "" {
			msg.WriteString("Missing certificate file path. ")
			msg.WriteString("Please supply with the -C flag. ")
		}

		if keyFile == "" {
			msg.WriteString("Missing key file path. ")
			msg.WriteString("Please supply with the -K flag. ")
		}

		panic(msg.String())
	}
}

func walletPath() string {
	return configDir("wallets")
}

var (
	certFile              string
	keyFile               string
	localNetworkNodeCount int
	remote                bool
	tls                   bool
)

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
	Run: func(cmd *cobra.Command, args []string) {
		var key *ecdsa.PrivateKey
		var err error
		var group *gossip3types.NotaryGroup
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if localNetworkNodeCount > 0 {
			fmt.Printf("Setting up local network with %d nodes\n", localNetworkNodeCount)
			group = setupLocalNetwork(ctx, localNetworkNodeCount)
		} else {
			fmt.Println("Using remote network")
			gossip3remote.Start()
			key, err = crypto.GenerateKey()
			if err != nil {
				panic(fmt.Sprintf("error generating key: %v", err))
			}
			p2pHost, err := p2p.NewLibP2PHost(ctx, key, 0)
			if err != nil {
				panic(fmt.Sprintf("error setting up p2p host: %v", err))
			}
			fmt.Println("Bootstrapping node")
			p2pHost.Bootstrap(p2p.BootstrapNodes())
			fmt.Println("Waiting for bootstrap")
			p2pHost.WaitForBootstrap(1, 10*time.Second)
			fmt.Println("Bootstrapped!")

			gossip3remote.NewRouter(p2pHost)

			group = setupNotaryGroup(nil, bootstrapPublicKeys)
			group.SetupAllRemoteActors(&key.PublicKey)
		}
		walletStorage := walletPath()

		client := gossip3client.New(group)
		if localNetworkNodeCount > 0 {
			for _, signer := range group.AllSigners() {
				signer.Actor.Tell(&gossip3messages.StartGossip{})
			}
		}
		if tls {
			panicWithoutTLSOpts()
			walletrpc.ServeTLS(walletStorage, client, certFile, keyFile)
		} else {
			walletrpc.ServeInsecure(walletStorage, client)
		}
	},
}

func init() {
	rootCmd.AddCommand(rpcServerCmd)
	rpcServerCmd.Flags().IntVarP(&localNetworkNodeCount, "local-network", "l", 0, "Run local network with randomly generated keys, specifying number of nodes as argument. Mutually exlusive with bootstrap-*")
	rpcServerCmd.Flags().BoolVarP(&tls, "tls", "t", false, "Encrypt connections with TLS/SSL")
	rpcServerCmd.Flags().StringVarP(&certFile, "tls-cert", "C", "", "TLS certificate file")
	rpcServerCmd.Flags().StringVarP(&keyFile, "tls-key", "K", "", "TLS private key file")
}
