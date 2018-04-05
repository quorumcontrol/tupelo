package dag

import (
	"testing"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/ipfs/go-cid"
	u "github.com/ipfs/go-ipfs-util"
	mh "github.com/multiformats/go-multihash"

	"github.com/stretchr/testify/assert"
	"github.com/davecgh/go-spew/spew"
	"github.com/ipfs/go-ipld-format"
)

type TestStruct struct {
	Name string
	Bar *cid.Cid
}

type TestLinks struct {
	Linkies []*cid.Cid
}

func TestLinkUnordered(t *testing.T) {
	c := cid.NewCidV1(cid.DagCBOR, u.Hash([]byte("something")))
	c2 := cid.NewCidV1(cid.DagCBOR, u.Hash([]byte("something")))

	node,err := cbornode.WrapObject(&TestLinks{
		Linkies: []*cid.Cid{c,c2},
	}, mh.SHA2_256, -1)
	assert.Nil(t, err)


	node2,err := cbornode.WrapObject(&TestLinks{
		Linkies: []*cid.Cid{c2,c},
	}, mh.SHA2_256, -1)
	assert.Nil(t, err)

	assert.Equal(t, node.Cid(), node2.Cid())

}

func TestCoins(t *testing.T) {
	c := cid.NewCidV1(cid.DagCBOR, u.Hash([]byte("something")))

	obj := map[string]interface{}{
		"name": "foo",
		"bar":  c,
	}

	obj2 := map[string]interface{}{
		"bar":  c,
		"name": "foo",
	}

	obj3 := &TestStruct{
		Name: "foo",
		Bar: c,
	}

	node,err := cbornode.WrapObject(obj, mh.SHA2_256, -1)
	assert.Nil(t, err)

	node2,err := cbornode.WrapObject(obj2, mh.SHA2_256, -1)
	assert.Nil(t, err)

	node3,err := cbornode.WrapObject(obj3, mh.SHA2_256, -1)

	assert.Equal(t, node.Cid(), node2.Cid(), node3.Cid())

	assert.Len(t, node.Links(), 1)

	lnk, _, err := node.ResolveLink([]string{"bar"})
	assert.Nil(t, err)
	t.Log(lnk.Cid.String())
	t.Log(spew.Sdump(lnk))

	t.Log(spew.Sdump(format.MakeLink(node)))
}