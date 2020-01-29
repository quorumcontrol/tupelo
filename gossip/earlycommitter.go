package gossip

import (
	"github.com/ipfs/go-cid"
	"sync"
)

type signerIDToCheckpoint map[string]cid.Cid
type signerList map[string]struct{}
type checkpointVotes map[cid.Cid]signerList

type earlyCommitter struct {
	sync.RWMutex

	currentSignerVotes signerIDToCheckpoint // map of signer id to its current vote (checkpoint CID)
	checkpointVotes    checkpointVotes      // map of checkpointCID to a list of signers voting for it
}

func newEarlyCommitter() *earlyCommitter {
	return &earlyCommitter{
		currentSignerVotes: make(signerIDToCheckpoint),
		checkpointVotes:    make(checkpointVotes),
	}
}

func (ec *earlyCommitter) Vote(signerID string, checkpointID cid.Cid) {
	ec.Lock()
	defer ec.Unlock()

	existing, ok := ec.currentSignerVotes[signerID]
	if ok {
		if existing == checkpointID {
			return
		}
		delete(ec.checkpointVotes[existing], signerID)
	}
	ec.currentSignerVotes[signerID] = checkpointID

	voteList, ok := ec.checkpointVotes[checkpointID]
	if !ok {
		voteList = make(signerList)
	}
	voteList[signerID] = struct{}{}
	ec.checkpointVotes[checkpointID] = voteList
}

func (ec *earlyCommitter) HasThreshold(totalCount int, threshold float64) (bool, cid.Cid) {
	ec.RLock()
	defer ec.RUnlock()

	total := float64(totalCount)
	for checkpointID, list := range ec.checkpointVotes {
		if float64(len(list))/total >= threshold {
			return true, checkpointID
		}
	}
	return false, cid.Undef
}
