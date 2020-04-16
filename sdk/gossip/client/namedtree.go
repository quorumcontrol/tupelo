package client

import (
	"context"
	"crypto/ecdsa"
	"strings"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/quorumcontrol/tupelo/sdk/consensus"
)

type NamedChainTreeGenerator struct {
	Namespace string
	Client    *Client
}

type NamedChainTreeOptions struct {
	Name   string
	Owners []string
}

func NewGenerator(client *Client, namespace string) NamedChainTreeGenerator {
	return NamedChainTreeGenerator{
		Client:    client,
		Namespace: namespace,
	}
}

// GenesisKey creates a new named key creation key based on the supplied name.
// It lower-cases the name first to ensure that chaintree names are case
// insensitive.
func (g *NamedChainTreeGenerator) GenesisKey(name string) (*ecdsa.PrivateKey, error) {
	// TODO: If we ever want case sensitivity, consider adding a bool flag
	// to the Generator or adding a new fun.
	lowerCased := strings.ToLower(name)
	return consensus.PassPhraseKey([]byte(lowerCased), []byte(g.Namespace))
}

func (g *NamedChainTreeGenerator) Create(ctx context.Context, opts *NamedChainTreeOptions) (*consensus.SignedChainTree, error) {
	gKey, err := g.GenesisKey(opts.Name)
	if err != nil {
		return nil, err
	}

	chainTree, err := consensus.NewSignedChainTree(ctx, gKey.PublicKey, g.Client.DagStore())
	if err != nil {
		return nil, err
	}

	setOwnershipTxn, err := chaintree.NewSetOwnershipTransaction(opts.Owners)
	if err != nil {
		return nil, err
	}

	_, err = g.Client.PlayTransactions(ctx, chainTree, gKey, []*transactions.Transaction{setOwnershipTxn})
	if err != nil {
		return nil, err
	}

	return chainTree, nil
}

func (g *NamedChainTreeGenerator) Did(name string) (string, error) {
	gKey, err := g.GenesisKey(name)
	if err != nil {
		return "", err
	}

	return consensus.EcdsaPubkeyToDid(gKey.PublicKey), nil
}

func (g *NamedChainTreeGenerator) Find(ctx context.Context, name string) (*consensus.SignedChainTree, error) {
	did, err := g.Did(name)
	if err != nil {
		return nil, err
	}

	return g.Client.GetLatest(ctx, did)
}
