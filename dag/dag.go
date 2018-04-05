package dag

import (
	cid "github.com/ipfs/go-cid"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
)

type Tip map[string]interface{}

type Coins map[string]*cid.Cid

type Authentications []*consensuspb.PublicKey

type Authorizations []*consensuspb.Authorization

type Mints []*cid.Cid

type Sends []*cid.Cid

type Receives []*cid.Cid

type QcCoin struct {
	Mints []*consensuspb.MintCoinTransaction
	Sends []*consensuspb.SendCoinTransaction
	Receives []*consensuspb.ReceiveCoinTransaction
}


type QcTip struct {
	Id string
	Authentications Authentications
	Coins []*QcCoin
}