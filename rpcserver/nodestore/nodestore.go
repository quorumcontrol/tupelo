package nodestore

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"

	cid "github.com/ipfs/go-cid"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
)

type NodestoreService struct {
	sessions map[string]adapters.Adapter
	lock     *sync.Mutex
}

func NewNodestoreService() *NodestoreService {
	return &NodestoreService{
		sessions: make(map[string]adapters.Adapter),
		lock:     &sync.Mutex{},
	}
}

func (n *NodestoreService) Open(_ context.Context, req *OpenRequest) (*Session, error) {
	adapterConfig, err := parseConfig(req.Config)
	if err != nil {
		return nil, fmt.Errorf("Error parsing config %v", err)
	}

	id := randomSessionID()

	n.lock.Lock()
	defer n.lock.Unlock()

	adapter, err := adapters.New(adapterConfig)
	if err != nil {
		return nil, err
	}

	n.sessions[id] = adapter

	return &Session{Id: id}, nil
}

func (n *NodestoreService) Close(_ context.Context, req *CloseRequest) (*ConfirmationResponse, error) {
	err := n.closeSession(req.Session)
	return &ConfirmationResponse{Success: (err != nil)}, err
}

func (n *NodestoreService) CloseAll(_ context.Context, req *CloseAllRequest) (*ConfirmationResponse, error) {
	errs := []error{}

	for id := range n.sessions {
		err := n.closeSession(&Session{Id: id})

		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return &ConfirmationResponse{Success: false}, fmt.Errorf("Error closing %d sessions: %v", len(errs), errs)
	}

	return &ConfirmationResponse{Success: true}, nil
}

func (n *NodestoreService) closeSession(session *Session) error {
	adapter, err := n.getSessionAdapter(session)
	if err != nil {
		return err
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	err = adapter.Close()
	if err != nil {
		return err
	}

	delete(n.sessions, session.Id)
	return nil
}

func (n *NodestoreService) Get(_ context.Context, req *GetRequest) (*GetResponse, error) {
	adapter, err := n.getSessionAdapter(req.Session)
	if err != nil {
		return nil, err
	}

	cid, err := cid.Cast(req.Cid.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Error casting CID %v", err)
	}

	node, err := adapter.Store().GetNode(cid)
	if err != nil {
		return nil, fmt.Errorf("Error getting node %v", err)
	}
	if node == nil {
		return nil, fmt.Errorf("Node does not exist")
	}

	return &GetResponse{
		Node: &Node{
			Bytes: node.RawData(),
		},
	}, nil
}

func (n *NodestoreService) Put(_ context.Context, req *PutRequest) (*PutResponse, error) {
	adapter, err := n.getSessionAdapter(req.Session)
	if err != nil {
		return nil, err
	}

	node, err := adapter.Store().CreateNodeFromBytes(req.Node.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Error putting node %v", err)
	}

	return &PutResponse{
		Cid: &Cid{
			Bytes: node.Cid().Bytes(),
		},
	}, nil
}

func (n *NodestoreService) Delete(_ context.Context, req *DeleteRequest) (*ConfirmationResponse, error) {
	adapter, err := n.getSessionAdapter(req.Session)
	if err != nil {
		return nil, err
	}

	cid, err := cid.Cast(req.Cid.Bytes)
	if err != nil {
		return &ConfirmationResponse{Success: false}, err
	}

	err = adapter.Store().DeleteNode(cid)
	if err != nil {
		return &ConfirmationResponse{Success: false}, fmt.Errorf("Error getting node %v", err)
	}

	return &ConfirmationResponse{Success: true}, nil
}

func (n *NodestoreService) getSessionAdapter(s *Session) (adapters.Adapter, error) {
	adapter, ok := n.sessions[s.Id]

	if !ok {
		return nil, fmt.Errorf("session %s does not exist", s.Id)
	}

	return adapter, nil
}

func parseConfig(c isOpenRequest_Config) (*adapters.Config, error) {
	switch config := c.(type) {
	case *OpenRequest_Badger:
		return &adapters.Config{
			Adapter: adapters.BadgerStorageAdapterName,
			Arguments: map[string]interface{}{
				"path": config.Badger.Path,
			},
		}, nil
	case *OpenRequest_Ipld:
		return &adapters.Config{
			Adapter: adapters.IpldStorageAdapterName,
			Arguments: map[string]interface{}{
				"path":    config.Ipld.Path,
				"address": config.Ipld.Address,
				"online":  !config.Ipld.Offline,
			},
		}, nil
	case *OpenRequest_Memory:
		return &adapters.Config{
			Adapter: adapters.MockStorageAdapterName,
			Arguments: map[string]interface{}{
				"namespace": config.Memory.Namespace,
			},
		}, nil
	default:
		return nil, fmt.Errorf("Unsupported storage adapter sepcified")
	}
}

func randomSessionID() string {
	return strconv.Itoa(rand.Intn(32))
}
