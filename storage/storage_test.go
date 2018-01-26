package storage_test

import (
	"testing"
	"github.com/quorumcontrol/qc3/did"
	"github.com/docker/docker/pkg/testutil/assert"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/attic-labs/noms/go/chunks"
	"github.com/attic-labs/noms/go/datas"
)

func cleanStorage() *storage.Storage {
	db := datas.NewDatabase((&chunks.MemoryStorage{}).NewView())
	return &storage.Storage{
		Db: db,
	}
}

func TestStorage_UpsertDid(t *testing.T) {
	storage := cleanStorage()

	didDoc,_,err := did.Generate()
	assert.NilError(t, err)

	assert.NilError(t, storage.UpsertDid(*didDoc))

	ret,err := storage.GetDid(didDoc.Id)

	assert.NilError(t, err)

	assert.Equal(t, didDoc.Id, ret.Id)
}

