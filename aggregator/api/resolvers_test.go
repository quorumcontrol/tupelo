package api

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/gqltesting"
	"github.com/quorumcontrol/tupelo/aggregator"
	"github.com/quorumcontrol/tupelo/sdk/gossip/testhelpers"
	"github.com/stretchr/testify/require"
)

func TestSanity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := NewResolver(ctx, aggregator.NewMemoryStore())
	require.Nil(t, err)

	opts := []graphql.SchemaOpt{graphql.UseFieldResolvers()}
	schema, err := graphql.ParseSchema(Schema, r, opts...)
	require.Nil(t, err)

	abr := testhelpers.NewValidTransaction(t)
	bits, err := abr.Marshal()
	require.Nil(t, err)
	abrString := base64.StdEncoding.EncodeToString(bits)
	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	abr2 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/my/path", "hi")
	did := string(abr2.ObjectId)

	// add abr2 manually so we can resolve
	_, err = r.Aggregator.Add(ctx, &abr2)
	require.Nil(t, err)

	gqltesting.RunTests(t, []*gqltesting.Test{
		{
			Schema: schema,
			Query: `
			mutation addBlock($addBlockRequest: String!) {
				addBlock(input: {addBlockRequest: $addBlockRequest}) {
					valid
				}
			}
			`,
			Variables: map[string]interface{}{
				"addBlockRequest": abrString,
			},
			ExpectedResult: `
				{
					"addBlock":{"valid":true}
				}
			`,
		},
		{
			Schema: schema,
			Query: `
			query resolve($did: String!, $path: String!) {
				resolve(input: {did: $did, path: $path}) {
					remainingPath
					value
				}
			}
			`,
			Variables: map[string]interface{}{
				"did":  did,
				"path": "tree/data/my/path",
			},
			ExpectedResult: `
				{
					"resolve":{"value":"hi", "remainingPath": []}
				}
			`,
		},
	})
}
