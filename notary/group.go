package notary

import (
	"fmt"
	"math"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
)

// see https://golang.org/pkg/sort/#Slice
type byAddress []*consensuspb.PublicKey

func (a byAddress) Len() int      { return len(a) }
func (a byAddress) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byAddress) Less(i, j int) bool {
	return consensus.BlsVerKeyToAddress(a[i].PublicKey).Hex() < consensus.BlsVerKeyToAddress(a[j].PublicKey).Hex()
}

type Group struct {
	Id               string
	SortedPublicKeys []*consensuspb.PublicKey
}

func GroupFromChain(chain *consensuspb.Chain) *Group {
	return NewGroup(chain.Id, chain.Authentication.PublicKeys)
}

func GroupFromPublicKeys(keys []*consensuspb.PublicKey) *Group {
	group := NewGroup("", keys)
	group.Id = consensus.AddrToDid(group.Address().Hex())
	return group
}

func NewGroup(id string, keys []*consensuspb.PublicKey) *Group {
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

func (group *Group) VerifySignature(msg []byte, sig *consensuspb.Signature) (bool, error) {
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

func (group *Group) CombineSignatures(sigs []*consensuspb.Signature, memo []byte) (*consensuspb.Signature, error) {
	sigBytes := make([][]byte, 0)
	sigMap := sigsByCreator(sigs)

	signers := make([]bool, len(group.SortedPublicKeys))
	for i, pubKey := range group.SortedPublicKeys {
		sig, ok := sigMap[pubKey.Id]
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

	return &consensuspb.Signature{
		Creator:   group.Id,
		Signers:   signers,
		Signature: combinedBytes,
		Memo:      memo,
	}, nil
}

func (group *Group) ReplaceSignatures(block *consensuspb.Block) (*consensuspb.Block, error) {
	log.Debug("replacing signatures", "block", block)
	hsh, err := consensus.BlockToHash(block)
	if err != nil {
		return nil, fmt.Errorf("error hashing block")
	}

	combinedSig, err := group.CombineSignatures(block.Signatures, hsh.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error combining sig: %v", err)
	}

	verified, err := group.VerifySignature(hsh.Bytes(), combinedSig)
	if err != nil || !verified {
		return nil, fmt.Errorf("error combining sig (verified? %v): %v", verified, err)
	}

	sigsMap := sigsByCreator(block.Signatures)
	for _, publicKey := range group.SortedPublicKeys {
		delete(sigsMap, consensus.BlsVerKeyToAddress(publicKey.PublicKey).Hex())
	}

	newSigs := make([]*consensuspb.Signature, len(sigsMap)+1)
	i := 0
	for _, sig := range sigsMap {
		newSigs[i] = sig
		i++
	}
	newSigs[i] = combinedSig
	block.Signatures = newSigs

	var newTransactionSigs []*consensuspb.Signature

	for memo, transactionSigs := range consensus.TransactionSignaturesByMemo(block) {
		combinedSig, err := group.CombineSignatures(transactionSigs, hexutil.MustDecode(memo))
		if err != nil {
			return nil, fmt.Errorf("error combining transaction sigs: %v", err)
		}

		newTransactionSigs = append(newTransactionSigs, combinedSig)
	}

	block.TransactionSignatures = newTransactionSigs

	return block, nil
}

func (group *Group) IsBlockSigned(block *consensuspb.Block) (bool, error) {
	if len(block.Signatures) == 0 {
		return false, nil
	}
	sigs := sigsByCreator(block.Signatures)
	sig, ok := sigs[group.Id]
	if !ok {
		return false, nil
	}

	hsh, err := consensus.BlockToHash(block)
	if err != nil {
		return false, fmt.Errorf("error hashing block: %v", err)
	}

	return group.VerifySignature(hsh.Bytes(), sig)
}

func (group *Group) IsTransactionSigned(block *consensuspb.Block, transaction *consensuspb.Transaction) (bool, error) {
	if len(block.TransactionSignatures) == 0 {
		return false, nil
	}
	sigs := sigsByMemo(block.TransactionSignatures)
	sig, ok := sigs["tx:"+transaction.Id]
	if !ok {
		return false, nil
	}

	if sig.Creator != group.Id {
		return false, nil
	}

	hsh, err := consensus.TransactionToHash(transaction)
	if err != nil {
		return false, fmt.Errorf("error hashing block: %v", err)
	}

	return group.VerifySignature(hsh.Bytes(), sig)
}

func sigsByCreator(sigs []*consensuspb.Signature) map[string]*consensuspb.Signature {
	sigsByCreator := make(map[string]*consensuspb.Signature)
	for _, sig := range sigs {
		sigsByCreator[sig.Creator] = sig
	}
	return sigsByCreator
}

func sigsByMemo(sigs []*consensuspb.Signature) map[string]*consensuspb.Signature {
	sigsByMemo := make(map[string]*consensuspb.Signature)
	for _, sig := range sigs {
		sigsByMemo[string(sig.Memo)] = sig
	}
	return sigsByMemo
}
