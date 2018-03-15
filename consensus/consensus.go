package consensus

import (
	"fmt"
	"strings"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

func AddrToDid (addr string) string {
	return fmt.Sprintf("did:qc:%s", addr)
}

func DidToAddr(did string) string {
	segs := strings.Split(did, ":")
	return segs[len(segs) - 1]
}

func EncapsulateTransaction(transactionType consensuspb.Transaction_TransactionType, transaction proto.Message) *consensuspb.Transaction {
	encoded,_ := proto.Marshal(transaction)
	return &consensuspb.Transaction{
		Id: uuid.New().String(),
		Type: transactionType,
		Payload: encoded,
	}
}

func ChainFromEcdsaKey(key *ecdsa.PublicKey) *consensuspb.Chain {
	chainId := AddrToDid(crypto.PubkeyToAddress(*key).Hex())
	return &consensuspb.Chain{
		Id: chainId,
		Authentication: &consensuspb.Authentication{
			PublicKeys: []*consensuspb.PublicKey{
				{
					ChainId: chainId,
					Id: crypto.PubkeyToAddress(*key).Hex(),
					PublicKey: crypto.CompressPubkey(key),
					Type: consensuspb.Secp256k1,
				},
			},
		},
	}
}

func BlsVerKeyToAddress(pubBytes []byte) common.Address {
	return common.BytesToAddress(crypto.Keccak256(pubBytes)[12:])
}

func BlsKeyToPublicKey(key *bls.VerKey) (*consensuspb.PublicKey) {
	return &consensuspb.PublicKey{
			Id: BlsVerKeyToAddress(key.Bytes()).Hex(),
			PublicKey: key.Bytes(),
			Type: consensuspb.BLSGroupSig,
	}
}

func ChainToTip(chain *consensuspb.Chain) (*consensuspb.ChainTip) {
	var lastHash []byte
	if len(chain.Blocks) > 0 {
		hsh,err := BlockToHash(chain.Blocks[len(chain.Blocks) - 1])
		if err != nil {
			//should *really* never happen
			log.Crit("error hashing last block", "error", err)
		}
		lastHash = hsh.Bytes()
	}

	chainTip := &consensuspb.ChainTip{
		Id: chain.Id,
		LastHash: lastHash,
		Authentication: chain.Authentication,
		Authorizations: chain.Authorizations,
	}
	return chainTip
}
