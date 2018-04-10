package dag

import (
	"github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multihash"
	"github.com/ipfs/go-cid"
	"sync"
	"fmt"
	"github.com/ipfs/go-ipld-format"
)

type BidirectionalTree struct {
	Root *cid.Cid
	counter int
	nodesByStaticId map[int]*bidirectionalNode
	nodesByCid map[string]*bidirectionalNode
	mutex sync.Mutex
}

type ErrorCode struct {
	Code  int
	Memo string
}

func (e *ErrorCode) Error() string {
	return fmt.Sprintf("%d - %s", e.Code, e.Memo)
}

const (
	ErrMissingRoot = 0
	ErrMissingPath = 1
)

type nodeId int

type bidirectionalNode struct {
	parents []nodeId
	id nodeId
	node    *cbornode.Node
}

func NewUgTree() *BidirectionalTree {
	return &BidirectionalTree{
		counter: 0,
		nodesByStaticId: make(map[int]*bidirectionalNode),
		nodesByCid: make(map[string]*bidirectionalNode),
	}
}

func (bn *bidirectionalNode) Resolve(tree *BidirectionalTree, path []string) (interface{}, []string, error) {
	val, remaining, err := bn.node.Resolve(path)
	if err != nil {
		return nil, nil, fmt.Errorf("error resolving: %v", err)
	}

	switch val.(type) {
	case *format.Link:
		n,ok := tree.nodesByCid[val.(*format.Link).Cid.KeyString()]
		if ok {
			return n.Resolve(tree, remaining)
		} else {
			return nil, nil, &ErrorCode{Code: ErrMissingPath}
		}
	default:
		return val, remaining, err
	}
}

func (ut *BidirectionalTree) Initialize(nodes ...*cbornode.Node) {
	ut.mutex.Lock()
	defer ut.mutex.Unlock()

	for i,node := range nodes {
		bidiNode := &bidirectionalNode{
			node: node,
			id: nodeId(i),
			parents: make([]nodeId,0),
		}
		ut.nodesByStaticId[i] = bidiNode
		ut.nodesByCid[node.Cid().KeyString()] = bidiNode
	}
	ut.counter = len(nodes)

	for _,bidiNode := range ut.nodesByStaticId {
		links := bidiNode.node.Links()
		for _,link := range links {
			existing,ok := ut.nodesByCid[link.Cid.KeyString()]
			if ok {
				bidiNode.parents = append(bidiNode.parents, existing.id)
			}
		}
	}
}

func (ut *BidirectionalTree)  Resolve(path []string) (interface{}, []string, error) {
	root,ok := ut.nodesByCid[ut.Root.KeyString()]
	if !ok {
		return nil, nil, &ErrorCode{Code: ErrMissingRoot}
	}
	return root.Resolve(ut, path)
}


type SafeWrap struct {
	Err error
}

func (sf *SafeWrap) WrapObject(obj interface{}) *cbornode.Node {
	if sf.Err != nil {
		return nil
	}
	node,err := cbornode.WrapObject(obj, multihash.SHA2_256, -1)
	sf.Err = err
	return node
}

