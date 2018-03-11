package notary

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/ethereum/go-ethereum/common"
	"sort"
	"fmt"
	"math"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/bls"
)


// see https://golang.org/pkg/sort/#Slice
type byAddress []*consensuspb.PublicKey
func (a byAddress) Len() int { return len(a) }
func (a byAddress) Swap(i,j int) { a[i], a[j] = a[j], a[i] }
func (a byAddress) Less(i,j int) bool { return common.BytesToAddress(a[i].PublicKey).Hex() < common.BytesToAddress(a[j].PublicKey).Hex() }

type Group struct {
	Id string
	SortedPublicKeys []*consensuspb.PublicKey
}

func NewGroup(id string, keys []*consensuspb.PublicKey) *Group {
	sort.Sort(byAddress(keys))
	return &Group{
		Id: id,
		SortedPublicKeys: keys,
	}
}

func (g *Group) Address() string {
	pubKeys := make([][]byte, len(g.SortedPublicKeys))
	for i,pubKey := range g.SortedPublicKeys {
		pubKeys[i] = pubKey.PublicKey
	}

	return common.BytesToAddress(concatBytes(pubKeys)).Hex()
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


func (group *Group) VerifySignature(block *consensuspb.Block, sig *consensuspb.Signature) (bool,error) {
	hsh,err := consensus.BlockToHash(block)
	if err != nil {
		return false, fmt.Errorf("error generating hash: %v", err)
	}

	requiredNum := uint64(math.Ceil(2.0 * (float64(len(group.SortedPublicKeys)) / 3.0)))

	var expectedKeyBytes [][]byte
	for i,didSign := range sig.Signers {
		if didSign {
			expectedKeyBytes = append(expectedKeyBytes, group.SortedPublicKeys[i].PublicKey)
		}
	}

	if uint64(len(expectedKeyBytes)) < requiredNum {
		return false,nil
	}

	return bls.VerifyMultiSig(sig.Signature, hsh.Bytes(), expectedKeyBytes)
}


func (group *Group) CombineSignatures(sigs []*consensuspb.Signature) (*consensuspb.Signature,error) {
	sigBytes := make([][]byte, len(sigs))

	for i,sig := range sigs {
		sigBytes[i] = sig.Signature
	}

	combinedBytes,err := bls.SumSignatures(sigBytes)
	if err != nil {
		return nil, fmt.Errorf("error summing sigs: %v", err)
	}

	sigsByCreator := make(map[string]*consensuspb.Signature)
	for _,sig := range sigs {
		sigsByCreator[sig.Creator] = sig
	}

	signers := make([]bool, len(group.SortedPublicKeys))
	for i,pubKey := range group.SortedPublicKeys {
		_,ok := sigsByCreator[pubKey.Id]
		if ok {
			signers[i] = true
		} else {
			signers[i] = false
		}
	}

	fmt.Printf("signers: %v", signers)

	return &consensuspb.Signature{
		Creator: group.Id,
		Signers: signers,
		Signature: combinedBytes,
	}, nil
}