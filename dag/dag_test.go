package dag

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"
)

type TestStruct struct {
	Name string
	Bar  *cid.Cid
}

type TestLinks struct {
	Linkies []*cid.Cid
}

func TestCreating(t *testing.T) {
	sw := &SafeWrap{}
	child := sw.WrapObject(map[string]interface{}{
		"name": "child",
	})

	root := sw.WrapObject(map[string]interface{}{
		"child": child.Cid(),
	})

	assert.Nil(t, sw.Err)

	tree := NewUgTree()
	tree.Initialize(root, child)
	tree.Tip = root.Cid()
}

func TestBidirectionalTree_Resolve(t *testing.T) {
	sw := &SafeWrap{}
	child := sw.WrapObject(map[string]interface{}{
		"name": "child",
	})

	root := sw.WrapObject(map[string]interface{}{
		"child": child.Cid(),
	})

	assert.Nil(t, sw.Err)

	tree := NewUgTree()
	tree.Initialize(root, child)
	tree.Tip = root.Cid()

	val, remaining, err := tree.Resolve([]string{"child", "name"})
	assert.Nil(t, err)
	assert.Empty(t, remaining)
	assert.Equal(t, "child", val)
}

func TestBidirectionalTree_Swap(t *testing.T) {
	sw := &SafeWrap{}
	child := sw.WrapObject(map[string]interface{}{
		"name": "child",
	})

	root := sw.WrapObject(map[string]interface{}{
		"child": child.Cid(),
	})

	assert.Nil(t, sw.Err)

	tree := NewUgTree()
	tree.Initialize(root, child)
	tree.Tip = root.Cid()

	newChild := sw.WrapObject(map[string]interface{}{
		"name": "newChild",
	})

	err := tree.Swap(child.Cid(), newChild)
	assert.Nil(t, err)

	val, remaining, err := tree.Resolve([]string{"child", "name"})
	assert.Nil(t, err)
	assert.Empty(t, remaining)
	assert.Equal(t, "newChild", val)

	err = tree.Swap(newChild.Cid(), child)
	assert.Nil(t, err)

	val, remaining, err = tree.Resolve([]string{"child", "name"})
	assert.Nil(t, err)
	assert.Empty(t, remaining)
	assert.Equal(t, "child", val)
}

func BenchmarkBidirectionalTree_Swap(b *testing.B) {
	sw := &SafeWrap{}
	child := sw.WrapObject(map[string]interface{}{
		"name": "child",
	})

	root := sw.WrapObject(map[string]interface{}{
		"child": child.Cid(),
	})

	tree := NewUgTree()
	tree.Initialize(root, child)
	tree.Tip = root.Cid()

	newChild := sw.WrapObject(map[string]interface{}{
		"name": "newChild",
	})

	swapper := []*cbornode.Node{child, newChild}

	var err error

	// run the Fib function b.N times
	for n := 0; n < b.N; n++ {
		idx := n % 2
		err = tree.Swap(swapper[idx].Cid(), swapper[(idx+1)%2])
	}

	assert.Nil(b, err)
}
