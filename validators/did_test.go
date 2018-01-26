package validators_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/attic-labs/noms/go/datas"
	"github.com/attic-labs/noms/go/chunks"
	"github.com/quorumcontrol/qc3/validators"
	"github.com/quorumcontrol/qc3/did"
)


func cleanStorage() *storage.Storage {
	db := datas.NewDatabase((&chunks.MemoryStorage{}).NewView())
	return &storage.Storage{
		Db: db,
	}
}

type SetupFunc func() (*storage.Storage, *did.Did, error)

func TestValidateDidInsert(t *testing.T) {
	for _,test := range []struct{
		Description string
		Setup SetupFunc
		ShouldValidate bool
	} {
		{
			Description: "valid, new Did",
			ShouldValidate: true,
			Setup: func() (*storage.Storage, *did.Did, error) {
				storage := cleanStorage()
				did,_,_ := did.Generate()
				return storage,did,nil
			},
		},
		{
			Description: "an existing Did",
			ShouldValidate: false,
			Setup: func() (*storage.Storage, *did.Did, error) {
				storage := cleanStorage()
				did,_,_ := did.Generate()
				storage.UpsertDid(*did)
				return storage,did,nil
			},
		},
	} {
		storage,did,err := test.Setup()
		assert.NoError(t, err, test.Description)

		isValid,err := validators.ValidateDidInsert(storage, *did)
		assert.NoError(t, err, test.Description)

		assert.Equal(t, test.ShouldValidate, isValid, test.Description)
	}
}
