package validators_test

import (
	"testing"
	"golang.org/x/crypto/blake2b"
	"github.com/btcsuite/btcutil/base58"
)

func pathToId(path string) string {
	hsh,err := blake2b.New256([]byte(path))
	if err != nil {
		panic("unexpected hash error")
	}
	return "down:" + base58.Encode(hsh.Sum(nil))
}

func TestIsValidOwnershipInsert(t *testing.T) {
	//genDid,genSecret,err := did.Generate()
	//assert.NoError(t, err)
	//
	//aliceDid,aliceSecret,err := did.Generate()
	//assert.NoError(t, err)
	//
	//genDescription := &ownership.OwnershipDescription{
	//	Context: "https://quorumcontrol.com/json-ld/ownership-description.json",
	//	Id: pathToId("/"),
	//	MinimumSignatures: 1,
	//	Owners: []string{aliceDid.Id},
	//	Did: genDid.Id,
	//}
	//
	//




}
