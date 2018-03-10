package notary

import (
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"context"
)

type TransactionValidator func(ctx context.Context, chain *internalchain.InternalChain, block *consensuspb.Block, transaction *consensuspb.Transaction) (bool,error)


func IsValidAddData(ctx context.Context, chain *internalchain.InternalChain, block *consensuspb.Block, transaction *consensuspb.Transaction) (bool,error) {
	return true, nil
}
