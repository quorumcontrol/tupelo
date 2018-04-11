package dag

import (
	"github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multihash"
	"github.com/ipfs/go-cid"
	"sync"
	"fmt"
	"github.com/ipfs/go-ipld-format"
	"errors"
	"encoding/json"
	"bytes"
)

type nodeId int

type BidirectionalTree struct {
	Tip             *cid.Cid
	counter         int
	nodesByStaticId map[nodeId]*bidirectionalNode
	nodesByCid      map[string]*bidirectionalNode
	mutex           sync.Mutex
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


type bidirectionalNode struct {
	parents []nodeId
	id nodeId
	node    *cbornode.Node
}

func NewUgTree() *BidirectionalTree {
	return &BidirectionalTree{
		counter: 0,
		nodesByStaticId: make(map[nodeId]*bidirectionalNode),
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
		ut.nodesByStaticId[nodeId(i)] = bidiNode
		ut.nodesByCid[node.Cid().KeyString()] = bidiNode
	}
	ut.counter = len(nodes)

	for _,bidiNode := range ut.nodesByStaticId {
		links := bidiNode.node.Links()
		for _,link := range links {
			existing,ok := ut.nodesByCid[link.Cid.KeyString()]
			if ok {
				existing.parents = append(existing.parents, bidiNode.id)
			}
		}
	}
}

func (ut *BidirectionalTree) Resolve(path []string) (interface{}, []string, error) {
	root,ok := ut.nodesByCid[ut.Tip.KeyString()]
	if !ok {
		return nil, nil, &ErrorCode{Code: ErrMissingRoot}
	}
	return root.Resolve(ut, path)
}

func (ut *BidirectionalTree) Swap(oldCid *cid.Cid, newNode *cbornode.Node) error {
	//fmt.Printf("swapping: %s \n", oldCid.String())

	existing,ok := ut.nodesByCid[oldCid.KeyString()]
	if !ok {
		return &ErrorCode{Code:ErrMissingPath, Memo: fmt.Sprintf("cannot find %s", oldCid.String())}
	}
	//fmt.Println("existing:")
	existing.dump()

	existingCid := existing.node.Cid()
	existing.node = newNode
	delete(ut.nodesByCid, existingCid.KeyString())

	ut.nodesByCid[newNode.Cid().KeyString()] = existing

	for _,parentId := range existing.parents {
		parent := ut.nodesByStaticId[parentId]
		//fmt.Println("parent")
		parent.dump()
		newParentJsonish := make(map[string]interface{})
		err := cbornode.DecodeInto(parent.node.RawData(), &newParentJsonish)
		if err != nil {
			return fmt.Errorf("error decoding: %v", err)
		}

		//fmt.Println("before:")
		//spew.Dump(newParentJsonish)

		err = updateLinks(newParentJsonish, existingCid, newNode.Cid())
		if err != nil {
			return fmt.Errorf("error updating links: %v", err)
		}

		//fmt.Println("after:")
		//spew.Dump(newParentJsonish)

		jBytes,_ := json.Marshal(newParentJsonish)

		newParentNode,err := cbornode.FromJson(bytes.NewReader(jBytes), multihash.SHA2_256, -1)
		if err != nil {
			return fmt.Errorf("error decoding: %v", err)
		}
		sw := &SafeWrap{}
		//fmt.Println("new parent node")
		obj := make(map[string]interface{})
		cbornode.DecodeInto(newParentNode.RawData(), &obj)
		//spew.Dump(obj)

		if sw.Err != nil {
			return fmt.Errorf("error wrapping object: %v", err)
		}

		if parent.node.Cid() == ut.Tip {
			ut.Tip = newParentNode.Cid()
		}

		ut.Swap(parent.node.Cid(), newParentNode)
	}

	//fmt.Println("after tree")
	ut.dump()

	return nil
}

func (bn *bidirectionalNode) dump() {
	//spew.Dump(bn)
	obj := make(map[string]interface{})
	cbornode.DecodeInto(bn.node.RawData(), &obj)
	//spew.Dump(obj)
}

func (bt *BidirectionalTree) dump() {
	//spew.Dump(bt)
	for _,n := range bt.nodesByStaticId {
		//fmt.Printf("node: %d", n.id)
		n.dump()
	}
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


func updateLinks(obj interface{}, oldCid *cid.Cid, newCid *cid.Cid) error {
	switch obj := obj.(type) {
		case map[interface{}]interface{}:
			for _, v := range obj {
				if err := updateLinks(v, oldCid, newCid); err != nil {
					return err
				}
			}
			return nil
		case map[string]interface{}:
			for ks, v := range obj {
				//fmt.Printf("k: %s\n", ks)

				if ks == "/" {
					vs, ok := v.(string)
					if ok {
						if vs == oldCid.String() {
							//fmt.Printf("updating link from %s to %s\n", oldCid.String(), newCid.String())
							obj[ks] = newCid.String()
						}
					} else {
						return errors.New("error, link was not a string")
					}
				} else {
					if err := updateLinks(v, oldCid, newCid); err != nil {
						return err
					}
				}
			}
			return nil
		case []interface{}:
			for _, v := range obj {
				if err := updateLinks(v, oldCid, newCid); err != nil {
					return err
				}
			}
			return nil
		default:
			return nil
		}
}


