package consensus

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/qc3/bls"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
)

func init() {
	typecaster.AddType(RemoteNode{})
	cbornode.RegisterCborType(RemoteNode{})
	typecaster.AddType(RoundInfo{})
	cbornode.RegisterCborType(RoundInfo{})
}

const (
	defaultRoundLength = 30 // round length in seconds
)

// NotaryGroup is a wrapper around a Chain Tree specifically used
// for keeping track of Signer membership and rewards.
type NotaryGroup struct {
	signedTree  *SignedChainTree
	ID          string
	RoundLength int // round length in seconds
}

// RoundInfo is a struct that holds information about the round.
// Currently it is only the signers, but is intended to also
// include rewards.
type RoundInfo struct {
	Signers []*RemoteNode
	Round   int64
}

func (ng *NotaryGroup) RoundAt(t time.Time) int64 {
	return t.UTC().Unix() / int64(ng.RoundLength)
}

// NewNotaryGroup takes an id and a nodeStore and creates a new empty NotaryGroup
func NewNotaryGroup(id string, nodeStore nodestore.NodeStore) *NotaryGroup {
	tree, err := chaintree.NewChainTree(NewEmptyTree(id, nodeStore), nil, DefaultTransactors)
	if err != nil {
		panic("error creating new tree")
	}
	return &NotaryGroup{
		signedTree: &SignedChainTree{
			ChainTree:  tree,
			Signatures: make(SignatureMap),
		},
		RoundLength: defaultRoundLength,
		ID:          id,
	}
}

// CreateGenesisState is used for creating a new notary group which fills in the initial
// signers for the first round
func (ng *NotaryGroup) CreateGenesisState(startRound int64, signers ...*RemoteNode) error {
	block := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				&chaintree.Transaction{
					Type: TransactionTypeSetData,
					Payload: &SetDataPayload{
						Path:  "rounds/" + strconv.Itoa(int(startRound)),
						Value: RoundInfo{Signers: signers},
					},
				},
			},
		},
	}

	return ng.AddBlock(block)
}

// MostRecentRound returns the roundinfo that is most recent to the requested round.
func (ng *NotaryGroup) MostRecentRoundInfo(round int64) (roundInfo *RoundInfo, err error) {
	for i := round; i >= 0; i-- {
		roundInfo, err = ng.RoundInfoFor(i)
		if err != nil {
			return nil, err
		}
		if roundInfo != nil {
			return
		}
	}
	return nil, fmt.Errorf("no valid round found")
}

func (ng *NotaryGroup) IsGenesis() bool {
	return ng.signedTree.IsGenesis()
}

// RoundInfoFor takes a round and returns the RoundInfo object
func (ng *NotaryGroup) RoundInfoFor(round int64) (roundInfo *RoundInfo, err error) {
	obj, _, err := ng.signedTree.ChainTree.Dag.Resolve([]string{"tree", "rounds", strconv.Itoa(int(round))})
	if err != nil {
		return nil, fmt.Errorf("error resolving round nodes: %v", err)
	}
	if obj == nil {
		return nil, nil
	}

	err = typecaster.ToType(obj, &roundInfo)
	if err != nil {
		return nil, fmt.Errorf("error casting resolved obj (%v): %v", obj, err)
	}
	roundInfo.Round = round
	return
}

// FastForward takes the notary group up to a known tip
func (ng *NotaryGroup) FastForward(tip *cid.Cid) error {
	ng.signedTree.ChainTree.Dag = ng.signedTree.ChainTree.Dag.WithNewTip(tip)
	return nil
}

// Tip returns the current tip of the notary group ChainTree
func (ng *NotaryGroup) Tip() *cid.Cid {
	return ng.signedTree.Tip()
}

func (ng *NotaryGroup) Dump() string {
	return ng.signedTree.ChainTree.Dag.Dump()
}

func (ng *NotaryGroup) AddNodes(nodes ...*cbornode.Node) error {
	return ng.signedTree.ChainTree.Dag.AddNodes(nodes...)
}

// CreateBlockFor takes a list of remoteNodes and a round and returns a block for gossipping
func (ng *NotaryGroup) CreateBlockFor(round int64, remoteNodes []*RemoteNode) (block *chaintree.BlockWithHeaders, err error) {
	var previousTip string
	if !ng.IsGenesis() {
		previousTip = ng.signedTree.Tip().String()
	}
	return &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: previousTip,
			Transactions: []*chaintree.Transaction{
				{
					Type: TransactionTypeSetData,
					Payload: &SetDataPayload{
						Path:  "rounds/" + strconv.Itoa(int(round)),
						Value: RoundInfo{Signers: remoteNodes},
					},
				},
			},
		},
	}, nil
}

// AddBlock takes a block and plays it against the NotaryGroup tree
func (ng *NotaryGroup) AddBlock(block *chaintree.BlockWithHeaders) (err error) {
	valid, err := ng.signedTree.ChainTree.ProcessBlock(block)
	if !valid || err != nil {
		return fmt.Errorf("error processing block (valid: %t): %v", valid, err)
	}
	return nil
}

// VerifyAvailableSignatures just validates that all the sigs are valid in the supplied argument,
// but does not verify that the super majority count has signed
func (ng *NotaryGroup) VerifyAvailableSignatures(round int64, msg []byte, sig *Signature) (bool, error) {
	return ng.VerifyThresholdSignatures(round, msg, sig, 0)
}

// VerifySignature makes sure over 2/3 of the signers in a particular round have approved a message
func (ng *NotaryGroup) VerifySignature(round int64, msg []byte, sig *Signature) (bool, error) {
	roundInfo, err := ng.MostRecentRoundInfo(round)
	if err != nil {
		return false, fmt.Errorf("error getting round info: %v", err)
	}

	requiredNum := roundInfo.SuperMajorityCount()
	log.Trace("verify signature", "requiredNum", requiredNum)

	return ng.VerifyThresholdSignatures(round, msg, sig, requiredNum)
}

// VerifyThresholdSignatures validates that all the sigs are valid in the supplied argument,
// and that at least a threshold number of signatures has signed
func (ng *NotaryGroup) VerifyThresholdSignatures(round int64, msg []byte, sig *Signature, threshold int64) (bool, error) {
	roundInfo, err := ng.MostRecentRoundInfo(round)
	if err != nil {
		return false, fmt.Errorf("error getting round info: %v", err)
	}
	var expectedKeyBytes [][]byte
	for i, didSign := range sig.Signers {
		if didSign {
			expectedKeyBytes = append(expectedKeyBytes, roundInfo.Signers[i].VerKey.PublicKey)
		}
	}

	log.Trace("verify signature", "len(expectedKeyBytes)", len(expectedKeyBytes))

	if int64(len(expectedKeyBytes)) < threshold {
		return false, nil
	}

	log.Trace("verify signature - verifying")

	return bls.VerifyMultiSig(sig.Signature, msg, expectedKeyBytes)
}

func (ng *NotaryGroup) GetNode(cid *cid.Cid) (*cbornode.Node, error) {
	return ng.signedTree.ChainTree.Dag.Get(cid)
}

func (ng *NotaryGroup) Nodes() ([]*cbornode.Node, error) {
	return ng.signedTree.ChainTree.Dag.Nodes()
}

func (ng *NotaryGroup) NodesAt(tip *cid.Cid) ([]*cbornode.Node, error) {
	dag := ng.signedTree.ChainTree.Dag.WithNewTip(tip)
	return dag.Nodes()
}

// ExpectedTipWithBlock returns what a notary group tip would be if it had the block
// without actually changing the notary group.
func (ng *NotaryGroup) ExpectedTipWithBlock(block *chaintree.BlockWithHeaders) (*cid.Cid, error) {
	newTree, err := chaintree.NewChainTree(ng.signedTree.ChainTree.Dag, ng.signedTree.ChainTree.BlockValidators, ng.signedTree.ChainTree.Transactors)
	if err != nil {
		return nil, fmt.Errorf("error creating new tree: %v", err)
	}
	valid, err := newTree.ProcessBlock(block)
	if !valid || err != nil {
		return nil, fmt.Errorf("error processing block (valid: %t): %v", valid, err)
	}
	return newTree.Dag.Tip, nil
}

// CombineSignatures turns many signatures into one for a particular round
// in the notary group.
func (ng *NotaryGroup) CombineSignatures(round int64, sigs SignatureMap) (*Signature, error) {
	roundInfo, err := ng.MostRecentRoundInfo(round)
	if err != nil {
		return nil, fmt.Errorf("error getting round info: %v", err)
	}
	log.Debug("combinging signatures", "signerLength", len(roundInfo.Signers))
	sigKeys := make([]string, 0)
	for sig := range sigs {
		sigKeys = append(sigKeys, sig)
	}
	log.Debug("signatureMap", "sigKeys", sigKeys)
	sigBytes := make([][]byte, 0)

	signers := make([]bool, len(roundInfo.Signers))
	for i, member := range roundInfo.Signers {
		sig, ok := sigs[member.Id]
		if ok {
			log.Trace("combine signatures, signer signed", "signerId", BlsVerKeyToAddress(member.VerKey.PublicKey).Hex())
			sigBytes = append(sigBytes, sig.Signature)
			signers[i] = true
		} else {
			log.Trace("signer not signed", "signerId", BlsVerKeyToAddress(member.VerKey.PublicKey).Hex())
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

// RandomMember returns a random signer from the RoundInfo
func (ri *RoundInfo) RandomMember() *RemoteNode {
	return ri.Signers[randInt(len(ri.Signers))]
}

// SuperMajorityCount returns the number needed for a consensus
func (ri *RoundInfo) SuperMajorityCount() int64 {
	required := int64((2.0 * float64(len(ri.Signers))) / 3.0)
	if required == 0 {
		return 1
	}
	return required
}

// HasMember will return true if provided verKey is in signers
func (ri *RoundInfo) HasMember(verKey *PublicKey) bool {
	for _, member := range ri.Signers {
		if bytes.Equal(member.VerKey.PublicKey, verKey.PublicKey) {
			return true
		}
	}
	return false
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
