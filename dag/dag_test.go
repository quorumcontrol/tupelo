package dag

import (
	"github.com/ipfs/go-cid"
	"testing"
	"github.com/stretchr/testify/assert"
)

type TestStruct struct {
	Name string
	Bar *cid.Cid
}

type TestLinks struct {
	Linkies []*cid.Cid
}


func TestCreating(t *testing.T) {
	sw := &SafeWrap{}
	child := sw.WrapObject(map[string]interface{} {
		"name": "child",
	})

	root := sw.WrapObject(map[string]interface{}{
		"child": child.Cid(),
	})

	assert.Nil(t, sw.Err)

	tree := NewUgTree()
	tree.Initialize(root,child)
	tree.Root = root.Cid()
}

func TestBidirectionalTree_Resolve(t *testing.T) {
	sw := &SafeWrap{}
	child := sw.WrapObject(map[string]interface{} {
		"name": "child",
	})

	root := sw.WrapObject(map[string]interface{}{
		"child": child.Cid(),
	})

	assert.Nil(t, sw.Err)

	tree := NewUgTree()
	tree.Initialize(root,child)
	tree.Root = root.Cid()

	val,remaining,err := tree.Resolve([]string{"child", "name"})
	assert.Nil(t, err)
	assert.Empty(t, remaining)
	assert.Equal(t, "child", val)
}
