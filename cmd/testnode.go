package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/remote"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/spf13/cobra"
)

var (
	BlsSignKeys  []*bls.SignKey
	EcdsaKeys    []*ecdsa.PrivateKey
	testnodePort int
)

// func bootstrapMembers(keys []*PublicKeySet) (members []*consensus.RemoteNode) {
// 	for _, keySet := range keys {
// 		blsPubKey := consensus.PublicKey{
// 			PublicKey: hexutil.MustDecode(keySet.BlsHexPublicKey),
// 			Type:      consensus.KeyTypeBLSGroupSig,
// 		}
// 		blsPubKey.Id = consensus.PublicKeyToAddr(&blsPubKey)

// 		ecdsaPubKey := consensus.PublicKey{
// 			PublicKey: hexutil.MustDecode(keySet.EcdsaHexPublicKey),
// 			Type:      consensus.KeyTypeSecp256k1,
// 		}
// 		ecdsaPubKey.Id = consensus.PublicKeyToAddr(&ecdsaPubKey)

// 		members = append(members, consensus.NewRemoteNode(blsPubKey, ecdsaPubKey))
// 	}

// 	return members
// }

// testnodeCmd represents the testnode command
var testnodeCmd = &cobra.Command{
	Use:   "test-node [index of key]",
	Short: "Run a testnet node with hardcoded (insecure) keys",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		logging.SetLogLevel("gossip", "ERROR")
		ecdsaKeyHex := os.Getenv("NODE_ECDSA_KEY_HEX")
		blsKeyHex := os.Getenv("NODE_BLS_KEY_HEX")
		signer := setupGossipNode(ctx, ecdsaKeyHex, blsKeyHex, "distributed-network", testnodePort)
		// signer.Host.Bootstrap(p2p.BootstrapNodes())
		signer.Actor.Tell(&messages.StartGossip{})
		stopOnSignal(signer)
	},
}

func setupNotaryGroup(local *types.Signer, keys []*PublicKeySet) *types.NotaryGroup {
	if len(keys) == 0 {
		panic(fmt.Sprintf("no keys found"))
	}
	group := types.NewNotaryGroup("hardcodedprivatekeysareunsafe")
	//TODO add members here
	if local != nil {
		group.AddSigner(local)
	}
	for _, keySet := range keys {
		ecdsaBytes := hexutil.MustDecode(keySet.EcdsaHexPublicKey)
		if local != nil && bytes.Equal(crypto.FromECDSAPub(local.DstKey), ecdsaBytes) {
			continue
		}

		verKeyBytes := hexutil.MustDecode(keySet.BlsHexPublicKey)

		signer := types.NewRemoteSigner(crypto.ToECDSAPub(ecdsaBytes), bls.BytesToVerKey(verKeyBytes))
		if local != nil {
			signer.Actor = actor.NewPID(signer.ActorAddress(local.DstKey), "tupelo-"+signer.ID)
		}
		group.AddSigner(signer)
	}
	return group
}

// func setupNotaryGroup(storageAdapter storage.Storage) *consensus.NotaryGroup {
// 	nodeStore := nodestore.NewStorageBasedStore(storageAdapter)
// 	group := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", nodeStore)
// 	if group.IsGenesis() {
// 		testNetMembers := bootstrapMembers(bootstrapPublicKeys)
// 		log.Debug("Creating gensis state", "nodes", len(testNetMembers))
// 		group.CreateGenesisState(group.RoundAt(time.Now()), testNetMembers...)
// 	}

// 	return group
// }

func setupGossipNode(ctx context.Context, ecdsaKeyHex string, blsKeyHex string, namespace string, port int) *types.Signer {
	remote.Start()

	ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
	if err != nil {
		panic("error fetching ecdsa key - set env variable NODE_ECDSA_KEY_HEX")
	}

	blsKey := bls.BytesToSignKey(hexutil.MustDecode(blsKeyHex))
	localSigner := types.NewLocalSigner(&ecdsaKey.PublicKey, blsKey)

	// id := consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey).Id
	log.Info("starting up a test node")

	storagePath := configDir(namespace).Path
	os.MkdirAll(storagePath, 0700)

	commitPath := filepath.Join(storagePath, localSigner.ID+"-commit")
	currentPath := filepath.Join(storagePath, localSigner.ID+"-current")
	badgerCommit, err := storage.NewBadgerStorage(commitPath)
	if err != nil {
		panic(fmt.Sprintf("error creating storage: %v", err))
	}
	badgerCurrent, err := storage.NewBadgerStorage(currentPath)
	if err != nil {
		panic(fmt.Sprintf("error creating storage: %v", err))
	}

	p2pHost, err := p2p.NewLibP2PHost(ctx, ecdsaKey, port)
	if err != nil {
		panic("error setting up p2p host")
	}
	p2pHost.Bootstrap(p2p.BootstrapNodes())
	log.Info("waiting for bootstrap", "nodes", p2p.BootstrapNodes())
	err = p2pHost.WaitForBootstrap(1, 10*time.Second)
	if err != nil {
		panic("timeout waiting for initial bootstrap")
	}
	log.Info("bootstrap complete")

	remote.NewRouter(p2pHost)

	group := setupNotaryGroup(localSigner, bootstrapPublicKeys)

	act, err := actor.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              localSigner,
		NotaryGroup:       group,
		CommitStore:       badgerCommit,
		CurrentStateStore: badgerCurrent,
	}), "tupelo-"+localSigner.ID)
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	localSigner.Actor = act
	return localSigner
}

func stopOnSignal(signers ...*types.Signer) {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Println(sig)
		for _, signer := range signers {
			log.Info("gracefully stopping signer")
			signer.Actor.GracefulStop()
		}
		done <- true
	}()
	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")
}

func init() {
	rootCmd.AddCommand(testnodeCmd)
	testnodeCmd.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")
	testnodeCmd.Flags().IntVarP(&testnodePort, "port", "p", 0, "what port will the node listen on")
}
