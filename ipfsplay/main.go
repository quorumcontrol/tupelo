package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	coreiface "github.com/ipsn/go-ipfs/core/coreapi/interface"
	opt "github.com/ipsn/go-ipfs/core/coreapi/interface/options"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/ipfsplay/ipfs"
)

func main() {

	logging.SetLogLevel("core", "debug")
	logging.SetLogLevel("blockstore", "debug")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dag, err := ipfs.StartIpfs(ctx)
	if err != nil {
		panic(fmt.Errorf("error starting ipfs: %v", err))
	}

	err = putUpATree(ctx, dag)
	if err != nil {
		panic(err)
	}
	select {}
}

func putUpATree(ctx context.Context, dag coreiface.DagAPI) error {
	treeKey, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("error generating key: %v", err)
	}
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	path := "some/data"
	value := "is now set"

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  path,
						"value": value,
					},
				},
			},
		},
	}
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	if err != nil {
		return fmt.Errorf("error new chain tree")
	}

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	if err != nil {
		return fmt.Errorf("error sign block")
	}
	testTree.ProcessBlock(blockWithHeaders)

	nodes, err := testTree.Dag.Nodes()
	if err != nil {
		return fmt.Errorf("error getting nodes")
	}

	batch := dag.Batch(ctx)
	for _, node := range nodes {
		path, err := batch.Put(ctx, bytes.NewReader(node.RawData()), opt.Dag.InputEnc("cbor"))
		if err != nil {
			return fmt.Errorf("error adding file: %v", err)
		}
		fmt.Printf("put %s\n", path)
	}
	err = batch.Commit(ctx)
	if err != nil {
		return fmt.Errorf("error committing: %v", err)
	}
	fmt.Printf("root: %s\n", testTree.Dag.Tip.String())
	return nil
}
