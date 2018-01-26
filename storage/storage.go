package storage

import (
	"github.com/attic-labs/noms/go/datas"
	"github.com/attic-labs/noms/go/types"
)

type Storage struct {
	Db datas.Database
}

type Root struct {
	DidDocs types.Map
	Capabilities types.Map
}

