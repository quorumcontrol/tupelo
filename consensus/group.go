package consensus

import (
	"fmt"
	"sort"

	"crypto/rand"
	"math/big"

	"sync"

	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/bls"
)

type byAddress []*RemoteNode

func (a byAddress) Len() int      { return len(a) }
func (a byAddress) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byAddress) Less(i, j int) bool {
	return a[i].Id < a[j].Id
}

type RemoteNode struct {
	VerKey PublicKey
	DstKey PublicKey
	Id     string
}

func NewRemoteNode(verKey, dstKey PublicKey) *RemoteNode {
	remoteNode := &RemoteNode{
		VerKey: verKey,
		DstKey: dstKey,
	}

	remoteNode.Id = PublicKeyToAddr(&verKey)
	return remoteNode
}

type Group struct {
	SortedMembers []*RemoteNode
	id            string
	lock          *sync.RWMutex
}

func NewGroup(members []*RemoteNode) *Group {
	sort.Sort(byAddress(members))
	return &Group{
		SortedMembers: members,
		lock:          new(sync.RWMutex),
	}
}

func (group *Group) AddMember(node *RemoteNode) error {
	group.lock.Lock()
	defer group.lock.Unlock()

	mapped := group.AsVerKeyMap()
	_, ok := mapped[node.Id]
	if ok {
		return nil
	}

	members := append(group.SortedMembers, node)
	sort.Sort(byAddress(members))
	group.SortedMembers = members
	return nil
}

func (group *Group) Id() string {
	if group.id != "" {
		return group.id
	}
	pubKeys := make([][]byte, len(group.SortedMembers))
	for i, remoteNode := range group.SortedMembers {
		pubKeys[i] = remoteNode.VerKey.PublicKey
	}

	group.id = BlsVerKeyToAddress(concatBytes(pubKeys)).String()

	return group.id
}

func (group *Group) AsVerKeyMap() map[string]PublicKey {
	sigMap := make(map[string]PublicKey)
	for _, member := range group.SortedMembers {
		sigMap[member.Id] = member.VerKey
	}
	return sigMap
}

func (group *Group) VerifySignature(msg []byte, sig *Signature) (bool, error) {
	requiredNum := group.SuperMajorityCount()
	log.Trace("verify signature", "requiredNum", requiredNum)

	var expectedKeyBytes [][]byte
	for i, didSign := range sig.Signers {
		if didSign {
			expectedKeyBytes = append(expectedKeyBytes, group.SortedMembers[i].VerKey.PublicKey)
		}
	}

	log.Trace("verify signature", "len(expectedKeyBytes)", len(expectedKeyBytes))

	if int64(len(expectedKeyBytes)) < requiredNum {
		return false, nil
	}

	log.Trace("verify signature - verifying")

	return bls.VerifyMultiSig(sig.Signature, msg, expectedKeyBytes)
}

func (group *Group) CombineSignatures(sigs SignatureMap) (*Signature, error) {
	sigBytes := make([][]byte, 0)

	signers := make([]bool, len(group.SortedMembers))
	for i, member := range group.SortedMembers {
		sig, ok := sigs[member.Id]
		if ok {
			log.Debug("combine signatures, signer signed", "signerId", BlsVerKeyToAddress(member.VerKey.PublicKey).Hex())
			sigBytes = append(sigBytes, sig.Signature)
			signers[i] = true
		} else {
			log.Debug("signer not signed", "signerId", BlsVerKeyToAddress(member.VerKey.PublicKey).Hex())
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

func (g *Group) RandomMember() *RemoteNode {
	return g.SortedMembers[randInt(len(g.SortedMembers))]
}

func (g *Group) SuperMajorityCount() int64 {
	required := int64((2.0 * float64(len(g.SortedMembers))) / 3.0)
	if required == 0 {
		return 1
	} else {
		return required
	}
}

func randInt(max int) int {
	bigInt, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		log.Error("error reading rand", "err", err)
	}
	return int(bigInt.Int64())
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
