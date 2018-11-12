package gossip2

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
)

func (cs CurrentState) Signable() []byte {
	return crypto.Keccak256(append(cs.ObjectID, cs.Tip...))
}

func (cs CurrentState) Verify(group *consensus.NotaryGroup) (bool, error) {
	roundInfo, err := group.MostRecentRoundInfo(group.RoundAt(time.Now()))
	if err != nil {
		return false, fmt.Errorf("error getting round info: %v", err)
	}
	if int64(len(cs.Signature.Signers)) < roundInfo.SuperMajorityCount() {
		return false, fmt.Errorf("invalid number of signatures")
	}

	var verKeys [][]byte
	for i, didSign := range cs.Signature.Signers {
		if didSign {
			verKeys = append(verKeys, roundInfo.Signers[i].VerKey.PublicKey)
		}
	}
	return bls.VerifyMultiSig(cs.Signature.Signature, cs.Signable(), verKeys)
}
