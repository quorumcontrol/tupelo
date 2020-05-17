package api

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/gqltesting"
	"github.com/quorumcontrol/tupelo/sdk/gossip/testhelpers"
	"github.com/stretchr/testify/require"
)

func TestSanity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := NewResolver(ctx)
	require.Nil(t, err)

	opts := []graphql.SchemaOpt{graphql.UseFieldResolvers()}
	schema, err := graphql.ParseSchema(Schema, r, opts...)
	require.Nil(t, err)

	abr := testhelpers.NewValidTransaction(t)
	bits, err := abr.Marshal()
	require.Nil(t, err)
	abrString := base64.StdEncoding.EncodeToString(bits)

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
	})
}
