package consensus

import (
	"fmt"
	"math"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/bls"
)

type byAddress []PublicKey

func (a byAddress) Len() int      { return len(a) }
func (a byAddress) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byAddress) Less(i, j int) bool {
	return BlsVerKeyToAddress(a[i].PublicKey).Hex() < BlsVerKeyToAddress(a[j].PublicKey).Hex()
}

type Group struct {
	Id               string
	SortedPublicKeys []PublicKey
}

func GroupFromPublicKeys(keys []PublicKey) *Group {
	group := NewGroup("", keys)
	group.Id = AddrToDid(group.Address().Hex())
	return group
}

func NewGroup(id string, keys []PublicKey) *Group {
	sort.Sort(byAddress(keys))
	return &Group{
		Id:               id,
		SortedPublicKeys: keys,
	}
}

func (group *Group) Address() common.Address {
	pubKeys := make([][]byte, len(group.SortedPublicKeys))
	for i, pubKey := range group.SortedPublicKeys {
		pubKeys[i] = pubKey.PublicKey
	}

	return BlsVerKeyToAddress(concatBytes(pubKeys))
}

func (group *Group) VerifySignature(msg []byte, sig *Signature) (bool, error) {
	requiredNum := uint64(math.Ceil(2.0 * (float64(len(group.SortedPublicKeys)) / 3.0)))
	log.Trace("verify signature", "requiredNum", requiredNum)

	var expectedKeyBytes [][]byte
	for i, didSign := range sig.Signers {
		if didSign {
			expectedKeyBytes = append(expectedKeyBytes, group.SortedPublicKeys[i].PublicKey)
		}
	}

	log.Trace("verify signature", "len(expectedKeyBytes)", len(expectedKeyBytes))

	if uint64(len(expectedKeyBytes)) < requiredNum {
		return false, nil
	}

	log.Trace("verify signature - verifying")

	return bls.VerifyMultiSig(sig.Signature, msg, expectedKeyBytes)
}

func (group *Group) CombineSignatures(sigs SignatureMap) (*Signature, error) {
	sigBytes := make([][]byte, 0)

	signers := make([]bool, len(group.SortedPublicKeys))
	for i, pubKey := range group.SortedPublicKeys {
		sig, ok := sigs[pubKey.Id]
		if ok {
			log.Debug("signer signed", "signerId", BlsVerKeyToAddress(pubKey.PublicKey).Hex())
			sigBytes = append(sigBytes, sig.Signature)
			signers[i] = true
		} else {
			log.Debug("signer not signed", "signerId", BlsVerKeyToAddress(pubKey.PublicKey).Hex())
			signers[i] = false
		}
	}

	combinedBytes, err := bls.SumSignatures(sigBytes)
	if err != nil {
		return nil, fmt.Errorf("error summing sigs: %v", err)
	}

	return &Signature{
		Signers:   signers,
		Signature: combinedBytes,
	}, nil
}

func concatBytes(slices [][]byte) []byte {
	var totalLen int
	for _, s := range slices {
		totalLen += len(s)
	}
	tmp := make([]byte, totalLen)
	var i int
	for _, s := range slices {
		i += copy(tmp[i:], s)
	}
	return tmp
}
