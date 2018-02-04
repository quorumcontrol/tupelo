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

	cap.Sign(*didDoc, didDoc.GetSigningKey().Id, secret.SecretSigningKey)

	assert.NotEmpty(t, cap.Proof)
}

func TestInvocation_Sign(t *testing.T) {
	didDoc,secret,err := did.Generate()
	assert.Nil(t,err)

	cap := &ownership.Capability{
		SignableCapability: ownership.SignableCapability{
			Context: "https://w3c-ccg.github.io/ld-ocap",
		},
	}

	assert.NoError(t, cap.Sign(*didDoc, didDoc.GetSigningKey().Id, secret.SecretSigningKey))
	assert.NotEmpty(t, cap.Proof)

	invocation := &ownership.Invocation{
		SignableInvocation: ownership.SignableInvocation{
			Type: "Invocation",
			Capability: *cap,
		},
	}

	assert.NoError(t, invocation.Sign(*didDoc, didDoc.GetSigningKey().Id, secret.SecretSigningKey))

	assert.NotEmpty(t, invocation.Proof)
}

func TestCapability_VerifyProof(t *testing.T) {
	defaultDidDoc,defaultSecret,err := did.Generate()
	assert.Nil(t,err)

	for _,test := range []struct{
		Description string
		Setup func() (ownership.Capability, ownership.Proof, did.Did)
		ShouldVerify bool
		ShouldError bool
	} {
		{
			Description: "a valid proof",
			ShouldVerify: true,
			ShouldError: false,
			Setup: func() (ownership.Capability, ownership.Proof, did.Did) {
				cap := &ownership.Capability{
					SignableCapability: ownership.SignableCapability{
						Context: "https://w3c-ccg.github.io/ld-ocap",
					},
				}

				assert.NoError(t, cap.Sign(*defaultDidDoc, defaultDidDoc.GetSigningKey().Id, defaultSecret.SecretSigningKey))
				assert.NotEmpty(t, cap.Proof)
				return *cap, *cap.Proof[0], *defaultDidDoc
			},
		},{
			Description: "a proof with a bad signature",
			ShouldVerify: false,
			ShouldError: false,
			Setup: func() (ownership.Capability, ownership.Proof, did.Did) {
				cap := &ownership.Capability{
					SignableCapability: ownership.SignableCapability{
						Context: "https://w3c-ccg.github.io/ld-ocap",
					},
				}
				assert.NoError(t, cap.Sign(*defaultDidDoc, defaultDidDoc.GetSigningKey().Id, defaultSecret.SecretSigningKey))
				assert.NotEmpty(t, cap.Proof)

				proof := cap.Proof[0]
				proof.SignatureValue = "badsignature"

				return *cap, *proof, *defaultDidDoc
			},
		},{
			Description: "a proof with a bad creator",
			ShouldVerify: false,
			ShouldError: true,
			Setup: func() (ownership.Capability, ownership.Proof, did.Did) {
				cap := &ownership.Capability{
					SignableCapability: ownership.SignableCapability{
						Context: "https://w3c-ccg.github.io/ld-ocap",
					},
				}
				assert.NoError(t, cap.Sign(*defaultDidDoc, defaultDidDoc.GetSigningKey().Id, defaultSecret.SecretSigningKey))
				assert.NotEmpty(t, cap.Proof)

				proof := cap.Proof[0]
				proof.Creator = "some other creator"

				return *cap, *proof, *defaultDidDoc
			},
		},
	} {
		capability,proof,didDoc := test.Setup()
		verified,err := capability.VerifyProof(proof, didDoc)
		if test.ShouldError {
			assert.Error(t, err, test.Description)
		} else {
			assert.NoError(t, err, test.Description)
		}
		assert.Equal(t, test.ShouldVerify, verified, test.Description)
	}
}

func TestInvocation_VerifyProof(t *testing.T) {
	defaultDidDoc,defaultSecret,err := did.Generate()
	assert.Nil(t,err)

	defaultCapability := &ownership.Capability{
		SignableCapability: ownership.SignableCapability{
			Context: "https://w3c-ccg.github.io/ld-ocap",
		},
	}
	assert.NoError(t, defaultCapability.Sign(*defaultDidDoc, defaultDidDoc.GetSigningKey().Id, defaultSecret.SecretSigningKey))
	assert.NotEmpty(t, defaultCapability.Proof)

	for _,test := range []struct{
		Description string
		Setup func() (ownership.Invocation, ownership.Proof, did.Did)
		ShouldVerify bool
		ShouldError bool
	} {
		{
			Description: "a valid proof",
			ShouldVerify: true,
			ShouldError: false,
			Setup: func() (ownership.Invocation, ownership.Proof, did.Did) {
				invocation := &ownership.Invocation{
					SignableInvocation: ownership.SignableInvocation{
						Type: "Invocation",
						Capability: *defaultCapability,
					},
				}
				assert.NoError(t, invocation.Sign(*defaultDidDoc, defaultDidDoc.GetSigningKey().Id, defaultSecret.SecretSigningKey))

				assert.NotEmpty(t, invocation.Proof)
				return *invocation, *invocation.Proof[0], *defaultDidDoc
			},
		},{
			Description: "a proof with a bad signature",
			ShouldVerify: false,
			ShouldError: false,
			Setup: func() (ownership.Invocation, ownership.Proof, did.Did) {
				invocation := &ownership.Invocation{
					SignableInvocation: ownership.SignableInvocation{
						Type: "Invocation",
						Capability: *defaultCapability,
					},
				}
				assert.NoError(t, invocation.Sign(*defaultDidDoc, defaultDidDoc.GetSigningKey().Id, defaultSecret.SecretSigningKey))

				assert.NotEmpty(t, invocation.Proof)
				proof := invocation.Proof[0]

				proof.SignatureValue = "badsignature"

				return *invocation, *invocation.Proof[0], *defaultDidDoc
			},
		},{
			Description: "a proof with a bad creator",
			ShouldVerify: false,
			ShouldError: true,
			Setup: func() (ownership.Invocation, ownership.Proof, did.Did) {
				invocation := &ownership.Invocation{
					SignableInvocation: ownership.SignableInvocation{
						Type: "Invocation",
						Capability: *defaultCapability,
					},
				}
				assert.NoError(t, invocation.Sign(*defaultDidDoc, defaultDidDoc.GetSigningKey().Id, defaultSecret.SecretSigningKey))

				assert.NotEmpty(t, invocation.Proof)
				proof := invocation.Proof[0]
				proof.Creator = "some other creator"

				return *invocation, *invocation.Proof[0], *defaultDidDoc
			},
		},
	} {
		invocation,proof,didDoc := test.Setup()
		verified,err := invocation.VerifyProof(proof, didDoc)
		if test.ShouldError {
			assert.Error(t, err, test.Description)
		} else {
			assert.NoError(t, err, test.Description)
		}
		assert.Equal(t, test.ShouldVerify, verified, test.Description)
	}
}
