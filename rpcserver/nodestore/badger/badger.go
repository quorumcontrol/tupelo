package badger

import (
	"github.com/quorumcontrol/chaintree/safewrap"
	"context"
	"fmt"
	format "github.com/ipfs/go-ipld-format"

	cid "github.com/ipfs/go-cid"
	chaintreeNodestore "github.com/quorumcontrol/chaintree/nodestore"
	tupelo "github.com/quorumcontrol/tupelo/rpcserver"
	"github.com/quorumcontrol/tupelo/rpcserver/nodestore"
	"github.com/quorumcontrol/tupelo/storage"
)

type BadgerNodestoreService struct {
	config    *Config
	nodestore chaintreeNodestore.DagStore
	cancel    context.CancelFunc
}

func NewBadgerNodestoreService(config *Config) (*BadgerNodestoreService, error) {
	ctx, cancel := context.WithCancel(context.TODO())

	if config.Path == "" {
		cancel()
		return nil, fmt.Errorf("Error initializing badger storage: missing path")
	}

	db, err := storage.NewDefaultBadger(config.Path)

	if err != nil {
		cancel()
		return nil, fmt.Errorf("Error initializing badger storage: %v", err)
	}
	store, err := chaintreeNodestore.FromDatastoreOffline(ctx, db)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error creating dag store: %v", err)
	}

	return &BadgerNodestoreService{
		config:    config,
		nodestore: store,
		cancel:    cancel,
	}, nil
}

func (s *BadgerNodestoreService) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *BadgerNodestoreService) Get(ctx context.Context, req *nodestore.GetRequest) (*nodestore.GetResponse, error) {
	cid, err := cid.Cast(req.Cid.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Error casting CID %v", err)
	}

	node, err := s.nodestore.Get(ctx, cid)
	if err != nil {
		return nil, fmt.Errorf("Error getting node %v", err)
	}
	if node == nil {
		return nil, fmt.Errorf("Node does not exist")
	}

	return &nodestore.GetResponse{
		Node: &tupelo.Node{
			Bytes: node.RawData(),
		},
	}, nil
}

func (s *BadgerNodestoreService) Put(ctx context.Context, req *nodestore.PutRequest) (*nodestore.PutResponse, error) {
	node, err := nodeFromBytes(req.Node.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Error creating node %v", err)
	}

	err = s.nodestore.Add(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("Error putting node %v", err)
	}

	return &nodestore.PutResponse{
		Cid: &tupelo.Cid{
			Bytes: node.Cid().Bytes(),
		},
	}, nil
}

func (s *BadgerNodestoreService) Delete(ctx context.Context, req *nodestore.DeleteRequest) (*nodestore.ConfirmationResponse, error) {
	cid, err := cid.Cast(req.Cid.Bytes)
	if err != nil {
		return &nodestore.ConfirmationResponse{Success: false}, err
	}

	err = s.nodestore.Remove(ctx, cid)
	if err != nil {
		return &nodestore.ConfirmationResponse{Success: false}, fmt.Errorf("Error getting node %v", err)
	}

	return &nodestore.ConfirmationResponse{Success: true}, nil
}

func nodeFromBytes(bits []byte) (format.Node, error) {
	sw := safewrap.SafeWrap{}
	n := sw.Decode(bits)
	return n, sw.Err
}