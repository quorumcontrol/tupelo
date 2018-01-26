package storage_test

import (
	"testing"
	"github.com/quorumcontrol/qc3/did"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/attic-labs/noms/go/chunks"
	"github.com/attic-labs/noms/go/datas"
	"github.com/quorumcontrol/qc3/ownership"
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
	assert.NoError(t, err)

	assert.NoError(t, storage.UpsertDid(*didDoc))

	ret,err := storage.GetDid(didDoc.Id)

	assert.NoError(t, err)

	assert.Equal(t, didDoc.Id, ret.Id)
}


func TestStorage_UpsertCapability(t *testing.T) {
	storage := cleanStorage()

	didDoc,secret,err := did.Generate()
	assert.NoError(t, err)

	cap := &ownership.Capability{
		SignableCapability: ownership.SignableCapability{
			Context: "https://w3c-ccg.github.io/ld-ocap",
		},
	}

	assert.NoError(t, cap.Sign(*didDoc, secret.SecretSigningKey))


	assert.NoError(t, storage.UpsertCapability(*cap))

	ret,err := storage.GetCapability(cap.Id)

	assert.NoError(t, err)

	assert.Equal(t, cap.Id, ret.Id)
}

