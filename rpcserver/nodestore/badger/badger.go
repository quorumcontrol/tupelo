package badger

import (
	"context"
	"fmt"

	cid "github.com/ipfs/go-cid"
	chaintreeNodestore "github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	tupelo "github.com/quorumcontrol/tupelo/rpcserver"
	"github.com/quorumcontrol/tupelo/rpcserver/nodestore"
)

type BadgerNodestoreService struct {
	config    *Config
	nodestore *chaintreeNodestore.StorageBasedStore
}

func NewBadgerNodestoreService(config *Config) (*BadgerNodestoreService, error) {
	if config.Path == "" {
		return nil, fmt.Errorf("Error initializing badger storage: missing path")
	}

	db, err := storage.NewBadgerStorage(config.Path)

	if err != nil {
		return nil, fmt.Errorf("Error initializing badger storage: %v", err)
	}

	return &BadgerNodestoreService{
		config:    config,
		nodestore: chaintreeNodestore.NewStorageBasedStore(db),
	}, nil
}

func (s *BadgerNodestoreService) Close() {
	s.nodestore.Close()
}

func (s *BadgerNodestoreService) Get(_ context.Context, req *nodestore.GetRequest) (*nodestore.GetResponse, error) {
	cid, err := cid.Cast(req.Cid.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Error casting CID %v", err)
	}

	node, err := s.nodestore.GetNode(cid)
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

func (s *BadgerNodestoreService) Put(_ context.Context, req *nodestore.PutRequest) (*nodestore.PutResponse, error) {
	node, err := s.nodestore.CreateNodeFromBytes(req.Node.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Error putting node %v", err)
	}

	return &nodestore.PutResponse{
		Cid: &tupelo.Cid{
			Bytes: node.Cid().Bytes(),
		},
	}, nil
}

func (s *BadgerNodestoreService) Delete(_ context.Context, req *nodestore.DeleteRequest) (*nodestore.ConfirmationResponse, error) {
	cid, err := cid.Cast(req.Cid.Bytes)
	if err != nil {
		return &nodestore.ConfirmationResponse{Success: false}, err
	}

	err = s.nodestore.DeleteNode(cid)
	if err != nil {
		return &nodestore.ConfirmationResponse{Success: false}, fmt.Errorf("Error getting node %v", err)
	}

	return &nodestore.ConfirmationResponse{Success: true}, nil
}
