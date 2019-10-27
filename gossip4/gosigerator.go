package gossip4

import (
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/messages/build/go/signatures"
)

type gosigerator struct {
	tips map[string]*signatures.TreeState
	notaryGroup      *types.NotaryGroup
	verKeys          []*bls.VerKey
	quorumCount      uint64
}


func (gs *gosigerator) validate(ctx context.Context, p peer.ID, msg proto.Message) bool {
	// decode the message into a TreeState
	treeState := &signatures.TreeState


	// compare the signer counts
	// if the new message is strictly better than what we have:
	//     replace ours with theirs
	// 	   return true
	// otherwise if it is strictly worse (we have all the same sigs)
	//     return false
	// if they have a new sig, combine ours
	//  	queue up a publish of the combination   
	//		return false
	//     
}