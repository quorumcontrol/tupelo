package nodebuilder

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
)

type LegacyPublicKeySet struct {
	BlsHexPublicKey   string `json:"blsHexPublicKey,omitempty"`
	EcdsaHexPublicKey string `json:"ecdsaHexPublicKey,omitempty"`
	PeerIDBase58Key   string `json:"peerIDBase58Key,omitempty"`
}

func (lpks *LegacyPublicKeySet) ToPublicKeySet() (*types.PublicKeySet, error) {
	blsBits := hexutil.MustDecode(lpks.BlsHexPublicKey)
	ecdsaBits := hexutil.MustDecode(lpks.EcdsaHexPublicKey)

	ecdsaPub, err := crypto.UnmarshalPubkey(ecdsaBits)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal ECDSA pub key: %v", err)
	}

	verKey := bls.BytesToVerKey(blsBits)

	return &types.PublicKeySet{
		DestKey: ecdsaPub,
		VerKey:  verKey,
	}, nil
}

type LegacyPrivateKeySet struct {
	BlsHexPrivateKey   string `json:"blsHexPrivateKey,omitempty"`
	EcdsaHexPrivateKey string `json:"ecdsaHexPrivateKey,omitempty"`
}

func (lpks *LegacyPrivateKeySet) ToPrivateKeySet() (*PrivateKeySet, error) {
	blsBits := hexutil.MustDecode(lpks.BlsHexPrivateKey)
	ecdsaBits := hexutil.MustDecode(lpks.EcdsaHexPrivateKey)

	ecdsaPrivate, err := crypto.ToECDSA(ecdsaBits)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal ECDSA private key: %v", err)
	}

	return &PrivateKeySet{
		DestKey: ecdsaPrivate,
		SignKey: bls.BytesToSignKey(blsBits),
	}, nil
}
