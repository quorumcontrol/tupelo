package consensus

import (
	"testing"

	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/stretchr/testify/assert"
)

type testSet struct {
	SignKeys  []*bls.SignKey
	VerKeys   []*bls.VerKey
	EcdsaKeys []*ecdsa.PrivateKey
	PubKeys   []PublicKey
}

func newTestSet(t *testing.T) *testSet {
	signKeys := blsKeys(5)
	verKeys := make([]*bls.VerKey, len(signKeys))
	pubKeys := make([]PublicKey, len(signKeys))
	ecdsaKeys := make([]*ecdsa.PrivateKey, len(signKeys))
	for i, signKey := range signKeys {
		ecdsaKey, _ := crypto.GenerateKey()
		verKeys[i] = signKey.MustVerKey()
		pubKeys[i] = BlsKeyToPublicKey(verKeys[i])
		ecdsaKeys[i] = ecdsaKey
	}

	return &testSet{
		SignKeys:  signKeys,
		VerKeys:   verKeys,
		PubKeys:   pubKeys,
		EcdsaKeys: ecdsaKeys,
	}
}

func groupFromTestSet(t *testing.T, set *testSet) *Group {
	members := make([]*RemoteNode, len(set.SignKeys))
	for i := range set.SignKeys {
		rn := NewRemoteNode(BlsKeyToPublicKey(set.VerKeys[i]), EcdsaToPublicKey(&set.EcdsaKeys[i].PublicKey))
		members[i] = rn
	}

	return NewGroup(members)
}

func blsKeys(size int) []*bls.SignKey {
	keys := make([]*bls.SignKey, size)
	for i := 0; i < size; i++ {
		keys[i] = bls.MustNewSignKey()
	}
	return keys
}

func TestGroupFromPublicKeys(t *testing.T) {
	ts := newTestSet(t)
	g := groupFromTestSet(t, ts)
	assert.IsType(t, &Group{}, g)
}

func TestGroup_CombineSignatures(t *testing.T) {
	ts := newTestSet(t)
	g := groupFromTestSet(t, ts)

	data := "somedata"

	sigs := make(SignatureMap)

	for i, signKey := range ts.SignKeys {
		sig, err := BlsSign(data, signKey)
		assert.Nil(t, err)
		sigs[ts.PubKeys[i].Id] = *sig
	}

	sig, err := g.CombineSignatures(sigs)
	assert.Nil(t, err)

	isVerified, err := g.VerifySignature(MustObjToHash(data), sig)
	assert.Nil(t, err)

	assert.True(t, isVerified)
}

func TestGroup_VerifySignature(t *testing.T) {
	ts := newTestSet(t)
	g := groupFromTestSet(t, ts)
	data := "somedata"

	for _, test := range []struct {
		description  string
		generator    func(t *testing.T) (sigs SignatureMap)
		shouldVerify bool
	}{
		{
			description: "a valid signature",
			generator: func(t *testing.T) (sigs SignatureMap) {
				sigs = make(SignatureMap)

				for i, signKey := range ts.SignKeys {
					sig, err := BlsSign(data, signKey)
					assert.Nil(t, err)
					sigs[ts.PubKeys[i].Id] = *sig
				}
				return sigs
			},
			shouldVerify: true,
		},
		{
			description: "with only one signer",
			generator: func(t *testing.T) (sigs SignatureMap) {
				sigs = make(SignatureMap)
				i := 0
				sig, err := BlsSign(data, ts.SignKeys[i])
				assert.Nil(t, err)
				sigs[ts.PubKeys[i].Id] = *sig
				return sigs
			},
			shouldVerify: false,
		},
	} {
		sigs := test.generator(t)
		sig, err := g.CombineSignatures(sigs)
		assert.Nil(t, err)
		isVerified, err := g.VerifySignature(MustObjToHash(data), sig)

		assert.Equal(t, test.shouldVerify, isVerified, test.description)
	}
}

func TestGroup_RandomMember(t *testing.T) {
	ts := newTestSet(t)
	g := groupFromTestSet(t, ts)

	assert.IsType(t, &RemoteNode{}, g.RandomMember())
}

func TestGroup_AsVerKeyeMap(t *testing.T) {
	ts := newTestSet(t)
	g := groupFromTestSet(t, ts)

	mapped := g.AsVerKeyMap()

	for _, member := range g.SortedMembers {
		mappedKey, ok := mapped[member.Id]
		assert.True(t, ok)
		assert.Equal(t, member.VerKey, mappedKey)
	}
}

func TestGroup_SuperMajorityCount(t *testing.T) {
	ts := newTestSet(t)
	g := groupFromTestSet(t, ts)
	assert.Equal(t, int64(3), g.SuperMajorityCount())
}
