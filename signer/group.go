package signer

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"math"
	"sort"
)

type byAddress []consensus.PublicKey

func (a byAddress) Len() int      { return len(a) }
func (a byAddress) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byAddress) Less(i, j int) bool {
	return consensus.BlsVerKeyToAddress(a[i].PublicKey).Hex() < consensus.BlsVerKeyToAddress(a[j].PublicKey).Hex()
}

type Group struct {
	Id               string
	SortedPublicKeys []consensus.PublicKey
}

func GroupFromPublicKeys(keys []consensus.PublicKey) *Group {
	group := NewGroup("", keys)
	group.Id = consensus.AddrToDid(group.Address().Hex())
	return group
}

func NewGroup(id string, keys []consensus.PublicKey) *Group {
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

	return consensus.BlsVerKeyToAddress(concatBytes(pubKeys))
}

func (group *Group) VerifySignature(msg []byte, sig *consensus.Signature) (bool, error) {
	requiredNum := uint64(math.Ceil(2.0 * (float64(len(group.SortedPublicKeys)) / 3.0)))

	var expectedKeyBytes [][]byte
	for i, didSign := range sig.Signers {
		if didSign {
			expectedKeyBytes = append(expectedKeyBytes, group.SortedPublicKeys[i].PublicKey)
		}
	}

	if uint64(len(expectedKeyBytes)) < requiredNum {
		return false, nil
	}

	return bls.VerifyMultiSig(sig.Signature, msg, expectedKeyBytes)
}

func (group *Group) CombineSignatures(sigs consensus.SignatureMap) (*consensus.Signature, error) {
	sigBytes := make([][]byte, 0)

	signers := make([]bool, len(group.SortedPublicKeys))
	for i, pubKey := range group.SortedPublicKeys {
		sig, ok := sigs[pubKey.Id]
		if ok {
			log.Debug("signer signed", "signerId", consensus.BlsVerKeyToAddress(pubKey.PublicKey).Hex())
			sigBytes = append(sigBytes, sig.Signature)
			signers[i] = true
		} else {
			log.Debug("signer not signed", "signerId", consensus.BlsVerKeyToAddress(pubKey.PublicKey).Hex())
			signers[i] = false
		}
	}

	combinedBytes, err := bls.SumSignatures(sigBytes)
	if err != nil {
		return nil, fmt.Errorf("error summing sigs: %v", err)
	}

	return &consensus.Signature{
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
