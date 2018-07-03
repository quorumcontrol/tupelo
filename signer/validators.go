package signer

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/qc3/consensus"
)

func (s *Signer) IsOwner(tree *dag.BidirectionalTree, blockWithHeaders *chaintree.BlockWithHeaders) (bool, chaintree.CodedError) {

	id, _, err := tree.Resolve([]string{"id"})
	if err != nil {
		return false, &consensus.ErrorCode{Memo: fmt.Sprintf("error: %v", err), Code: consensus.ErrUnknown}
	}

	stored, err := s.Storage.Get(DidBucket, []byte(id.(string)))

	if err != nil {
		return false, &consensus.ErrorCode{Memo: fmt.Sprintf("error getting storage: %v", err), Code: consensus.ErrUnknown}
	}

	headers := &consensus.StandardHeaders{}

	err = typecaster.ToType(blockWithHeaders.Headers, headers)
	if err != nil {
		return false, &consensus.ErrorCode{Memo: fmt.Sprintf("error: %v", err), Code: consensus.ErrUnknown}
	}

	var addrs []string

	if stored == nil {
		// this is a genesis block
		addrs = []string{consensus.DidToAddr(id.(string))}
	} else {

		storedTip, err := cid.Cast(stored)
		if err != nil {
			return false, &consensus.ErrorCode{Code: consensus.ErrUnknown, Memo: fmt.Sprintf("bad CID stored: %v", err)}
		}

		oldRoot := tree.Get(storedTip)
		if oldRoot == nil {
			return false, &consensus.ErrorCode{Code: consensus.ErrUnknown, Memo: "missing old root"}
		}

		uncastAuths, remain, err := oldRoot.Resolve(tree, []string{"tree", "_qc", "authentications"})
		if err != nil {
			if err.(*dag.ErrorCode).GetCode() == dag.ErrMissingPath {
				addrs = []string{consensus.DidToAddr(id.(string))}
			} else {
				return false, &consensus.ErrorCode{Code: consensus.ErrUnknown, Memo: fmt.Sprintf("err resolving: %v", err)}
			}
		} else {
			// if there is no _qc or no authentications then it's like a genesis block
			if len(remain) == 1 || len(remain) == 2 {
				addrs = []string{consensus.DidToAddr(id.(string))}
			} else {
				var authentications []*consensus.PublicKey
				err = typecaster.ToType(uncastAuths, &authentications)
				if err != nil {
					return false, &consensus.ErrorCode{Code: consensus.ErrUnknown, Memo: fmt.Sprintf("err casting: %v", err)}
				}

				addrs = make([]string, len(authentications))
				for i, key := range authentications {
					addrs[i] = consensus.PublicKeyToAddr(key)
				}
			}
		}
	}

	for _, addr := range addrs {
		isSigned, err := consensus.IsBlockSignedBy(blockWithHeaders, addr)

		if err != nil {
			return false, &consensus.ErrorCode{Memo: fmt.Sprintf("error finding if signed: %v", err), Code: consensus.ErrUnknown}
		}

		if isSigned {
			return true, nil
		}
	}

	return false, nil
}
