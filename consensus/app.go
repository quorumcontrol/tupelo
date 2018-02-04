package consensus

import (
	"github.com/tendermint/abci/types"
	"github.com/tendermint/iavl"
	dbm "github.com/tendermint/tmlibs/db"
	"fmt"
	"bytes"
	"github.com/tendermint/go-wire"
)

// Return codes for the examples
const (
	CodeTypeOK            uint32 = 0
	CodeTypeEncodingError uint32 = 1
	CodeTypeBadNonce      uint32 = 2
	CodeTypeUnauthorized  uint32 = 3

	CodeTypeBadOption uint32 = 101
)


var _ types.Application = (*BlockOwnerApp)(nil)

type BlockOwnerApp struct {
	types.BaseApplication
	state *iavl.VersionedTree
}

func NewBlockOwnerApp() *BlockOwnerApp {
	state := iavl.NewVersionedTree(0, dbm.NewMemDB())
	return &BlockOwnerApp{state: state}
}

func (app *BlockOwnerApp) Info(req types.RequestInfo) (resInfo types.ResponseInfo) {
	return types.ResponseInfo{Data: fmt.Sprintf("{\"size\":%v}", app.state.Size())}
}

// tx is either "key=value" or just arbitrary bytes
func (app *BlockOwnerApp) DeliverTx(tx []byte) types.ResponseDeliverTx {
	var key, value []byte
	parts := bytes.Split(tx, []byte("="))
	if len(parts) == 2 {
		key, value = parts[0], parts[1]
	} else {
		key, value = tx, tx
	}
	app.state.Set(key, value)

	tags := []*types.KVPair{
		{Key: "app.creator", ValueType: types.KVPair_STRING, ValueString: "jae"},
		{Key: "app.key", ValueType: types.KVPair_STRING, ValueString: string(key)},
	}
	return types.ResponseDeliverTx{Code: CodeTypeOK, Tags: tags}
}


func (app *BlockOwnerApp) CheckTx(tx []byte) types.ResponseCheckTx {
	return types.ResponseCheckTx{Code: CodeTypeOK}
}


func (app *BlockOwnerApp) Commit() types.ResponseCommit {
	// Save a new version
	var hash []byte
	var err error

	if app.state.Size() > 0 {
		// just add one more to height (kind of arbitrarily stupid)
		height := app.state.LatestVersion() + 1
		hash, err = app.state.SaveVersion(height)
		if err != nil {
			// if this wasn't a dummy app, we'd do something smarter
			panic(err)
		}
	}

	return types.ResponseCommit{Code: CodeTypeOK, Data: hash}
}


func (app *BlockOwnerApp) Query(reqQuery types.RequestQuery) (resQuery types.ResponseQuery) {
	if reqQuery.Prove {
		value, proof, err := app.state.GetWithProof(reqQuery.Data)
		// if this wasn't a dummy app, we'd do something smarter
		if err != nil {
			panic(err)
		}
		resQuery.Index = -1 // TODO make Proof return index
		resQuery.Key = reqQuery.Data
		resQuery.Value = value
		resQuery.Proof = wire.BinaryBytes(proof)
		if value != nil {
			resQuery.Log = "exists"
		} else {
			resQuery.Log = "does not exist"
		}
		return
	} else {
		index, value := app.state.Get(reqQuery.Data)
		resQuery.Index = int64(index)
		resQuery.Value = value
		if value != nil {
			resQuery.Log = "exists"
		} else {
			resQuery.Log = "does not exist"
		}
		return
	}
}
