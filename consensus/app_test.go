package consensus_test

import (
	"testing"
	"github.com/tendermint/abci/types"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/iavl"
	"github.com/quorumcontrol/qc3/consensus"
)

func testApp(t *testing.T, app types.Application, tx []byte, key, value string) {
	ar := app.DeliverTx(tx)
	require.False(t, ar.IsErr(), ar)
	// repeating tx doesn't raise error
	ar = app.DeliverTx(tx)
	require.False(t, ar.IsErr(), ar)

	// make sure query is fine
	resQuery := app.Query(types.RequestQuery{
		Path: "/store",
		Data: []byte(key),
	})
	require.Equal(t, consensus.CodeTypeOK, resQuery.Code)
	require.Equal(t, value, string(resQuery.Value))

	// make sure proof is fine
	resQuery = app.Query(types.RequestQuery{
		Path:  "/store",
		Data:  []byte(key),
		Prove: true,
	})
	require.EqualValues(t, consensus.CodeTypeOK, resQuery.Code)
	require.Equal(t, value, string(resQuery.Value))
	proof, err := iavl.ReadKeyExistsProof(resQuery.Proof)
	require.Nil(t, err)
	err = proof.Verify([]byte(key), resQuery.Value, proof.RootHash)
	require.Nil(t, err, "%+v", err) // NOTE: we have no way to verify the RootHash
}


func TestBlockOwnerApp(t *testing.T) {
	app := consensus.NewBlockOwnerApp()
	key := "abc"
	value := key
	tx := []byte(key)
	testApp(t, app, tx, key, value)

	value = "def"
	tx = []byte(key + "=" + value)
	testApp(t, app, tx, key, value)
}