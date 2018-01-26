package ownership_test

import (
	"testing"
	"github.com/quorumcontrol/qc3/did"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/qc3/ownership"
)

func TestCapability_Sign(t *testing.T) {
	didDoc,secret,err := did.Generate()
	assert.Nil(t,err)

	cap := &ownership.Capability{
		SignableCapability: ownership.SignableCapability{
			Context: "https://w3c-ccg.github.io/ld-ocap",
		},
	}

	cap.Sign(*didDoc, secret.SecretSigningKey)

	assert.NotEmpty(t, cap.Proof)

}
