package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/gqltesting"
	format "github.com/ipfs/go-ipld-format"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo/aggregator"
	"github.com/quorumcontrol/tupelo/sdk/gossip/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBlocks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStore := aggregator.NewMemoryStore()
	dagStore, err := nodestore.FromDatastoreOffline(ctx, memStore)
	require.Nil(t, err)

	r, err := NewResolver(ctx, memStore)
	require.Nil(t, err)

	opts := []graphql.SchemaOpt{graphql.UseFieldResolvers()}
	schema, err := graphql.ParseSchema(Schema, r, opts...)
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}
	b1 := sw.WrapObject("block1")
	b2 := sw.WrapObject("block2")
	dagStore.AddMany(ctx, []format.Node{b1, b2})

	gqltesting.RunTests(t, []*gqltesting.Test{
		{
			Schema: schema,
			Query: fmt.Sprintf(`
			query {
				blocks(input:{ids:["%s","%s"]}) {
				  blocks {
					cid
				  }
				}
			  }
			`, b1.Cid().String(), b2.Cid().String()),
			ExpectedResult: fmt.Sprintf(`
				{
					"blocks":{"blocks":[{"cid": "%s"},{"cid":"%s"}]}
				}
			`, b1.Cid().String(), b2.Cid().String()),
		},
	})

}

func TestTouchedNodes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := NewResolver(ctx, aggregator.NewMemoryStore())
	require.Nil(t, err)

	opts := []graphql.SchemaOpt{graphql.UseFieldResolvers()}
	schema, err := graphql.ParseSchema(Schema, r, opts...)
	require.Nil(t, err)

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	abr2 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/my/path", "hi")
	did := string(abr2.ObjectId)

	// add abr2 manually so we can resolve
	_, err = r.Aggregator.Add(ctx, &abr2)
	require.Nil(t, err)

	schemaResp := schema.Exec(ctx,
		`query resolve($did: String!, $path: String!) {
			resolve(input: {did: $did, path: $path}) {
				remainingPath
				value
				touchedBlocks {
					cid
				}
			}
		}
		`,
		"resolve",
		map[string]interface{}{
			"did":  did,
			"path": "tree/data/my/path",
		},
	)

	type Response struct {
		Resolve struct {
			TouchedBlocks []Block `json:"touchedBlocks"`
		} `json:"resolve"`
	}

	resp := &Response{}
	json.Unmarshal(schemaResp.Data, resp)
	assert.Len(t, resp.Resolve.TouchedBlocks, 4)
}

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
