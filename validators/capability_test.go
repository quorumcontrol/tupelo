package validators_test

import (
	"github.com/quorumcontrol/qc3/did"
	"github.com/quorumcontrol/qc3/validators"
	"testing"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/quorumcontrol/qc3/ownership"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

func TestIsValidCapability(t *testing.T) {
	type SetupFunc func() (*storage.Storage, *ownership.Capability, error)

	aliceDid,aliceSecret,err := did.Generate()
	assert.NoError(t, err)

	//bobDid,bobSecret,err := did.Generate()
	//assert.NoError(t, err)

	for _,test := range []struct{
		Description string
		Setup SetupFunc
		ShouldValidate bool
	} {
		{
			Description: "valid capability",
			ShouldValidate: true,
			Setup: func() (*storage.Storage, *ownership.Capability, error) {
				store := cleanStorage()
				cap := &ownership.Capability{
					SignableCapability: ownership.SignableCapability{
						Id: "dcap:" + bson.NewObjectId().Hex(),
						Type: "Capability",
						Target: ownership.QcTarget{
							Context: "https://quorumcontrol.com/json-ld/qc-target.json-ld",
							Type: "QcTarget",
							Asset: "/google/search",
							Action: "update",
						},
						AuthenticationMaterial: []did.PublicKey{*aliceDid.GetSigningKey()},
					},
				}
				cap.Sign(*aliceDid, aliceDid.GetSigningKey().Id,aliceSecret.SecretSigningKey)
				store.UpsertDid(*aliceDid)
				return store,cap,nil
			},
		},
	} {
		store,cap,err := test.Setup()
		assert.NoError(t, err, test.Description)

		isValid,err := validators.IsValidCapability(store,*cap)
		assert.NoError(t, err, test.Description)

		assert.Equal(t, test.ShouldValidate, isValid, test.Description)
	}
}

