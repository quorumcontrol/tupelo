package did_test

import (
	"testing"
	did2 "github.com/quorumcontrol/qc3/did"
	"github.com/stretchr/testify/assert"
	"regexp"
	"encoding/json"
)

func TestGenerate(t *testing.T) {
	did,secret,err := did2.Generate()
	assert.Nil(t, err)
	assert.Equal(t, 32, len(secret.SecretEncryptionKey))
	assert.Regexp(t, regexp.MustCompile("^did:qc:\\w{21,22}"), did.Id)
}

func TestMarshall(t *testing.T) {
	did,_,err := did2.Generate()
	assert.Nil(t,err)
	bytes,err := json.Marshal(did)
	assert.Nil(t,err)
	assert.True(t, len(bytes) > 0)
}

func TestDid_GetSigningKey(t *testing.T) {
	did,_,err := did2.Generate()
	assert.Nil(t,err)

	key := did.GetSigningKey()
	assert.Equal(t, did.PublicKey[0], *key)
}