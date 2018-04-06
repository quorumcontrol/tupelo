package dag

import (
	cid "github.com/ipfs/go-cid"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multihash"
	"fmt"
)

type Tip map[string]interface{}

type Coins []*cid.Cid

type Authentications []*consensuspb.PublicKey

type Authorizations []*consensuspb.Authorization

type Mints []*cid.Cid

type Sends []*cid.Cid

type Receives []*cid.Cid

type Coin struct {
	Mints Mints
	Sends Sends
	Receives Receives
}

type QcCoin struct {
	Mints []*consensuspb.MintCoinTransaction
	Sends []*consensuspb.SendCoinTransaction
	Receives []*consensuspb.ReceiveCoinTransaction
}

type QcTip struct {
	Id string
	Authentications Authentications
	Authorizations Authorizations
	Coins []*QcCoin
}

type safeWrap struct {
	err error
}

func (sf *safeWrap) WrapObject(obj interface{}) *cbornode.Node {
	if sf.err != nil {
		return nil
	}
	node,err := cbornode.WrapObject(obj, multihash.SHA2_256, -1)
	sf.err = err
	return node
}

func QcTipToDag(tip *QcTip) (map[*cid.Cid]*cbornode.Node,error) {
	cborTip := make(Tip)

	sf := new(safeWrap)
	cborAuthentications := sf.WrapObject(tip.Authentications)
	cborAuthorizations := sf.WrapObject(tip.Authentications)
	if sf.err != nil {
		return nil, fmt.Errorf("error wrapping: %v", sf.err)
	}


	coins := make(Coins, len(tip.Coins))
	for i,qcCoin := range tip.Coins {

		coin := &Coin{

		}
	}

}
