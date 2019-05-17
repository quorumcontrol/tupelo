package badger 

import (
	"context"
	"os"
	"net"
	"testing"

	"github.com/quorumcontrol/chaintree/safewrap"
	tupelo "github.com/quorumcontrol/tupelo/rpcserver"
	"github.com/quorumcontrol/tupelo/rpcserver/nodestore"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
)

func TestNodestore(t *testing.T) {
	path := ".tmp/badger"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(".tmp")

	n, err := NewBadgerNodestoreService(&Config{ Path: path })
	require.Nil(t, err)
	defer n.Close()

	server := grpc.NewServer()
	RegisterBadgerNodestoreServiceServer(server, n)

	listener, err := net.Listen("tcp", ":0")
	require.Nil(t, err)

	go func() {
		err := server.Serve(listener)
		require.Nil(t, err)
	}()
	defer server.Stop()

	conn, err := grpc.Dial(listener.Addr().String(), grpc.WithInsecure())
	require.Nil(t, err)
	defer conn.Close()

	sw := safewrap.SafeWrap{}
	obj := sw.WrapObject(map[string]interface{}{"key": "val"})

	ctx := context.Background()

	nodestoreClient := NewBadgerNodestoreServiceClient(conn)

	putResponse, err := nodestoreClient.Put(ctx, &nodestore.PutRequest{Node: &tupelo.Node{Bytes: obj.RawData()}})
	require.Nil(t, err)
	require.Equal(t, putResponse.Cid.Bytes, obj.Cid().Bytes())

	getResponse, err := nodestoreClient.Get(ctx, &nodestore.GetRequest{Cid: putResponse.Cid})
	require.Nil(t, err)
	require.Equal(t, getResponse.Node.Bytes, obj.RawData())

	deleteResponse, err := nodestoreClient.Delete(ctx, &nodestore.DeleteRequest{Cid: putResponse.Cid})
	require.Nil(t, err)
	require.Equal(t, deleteResponse.Success, true)

	_, err = nodestoreClient.Get(ctx, &nodestore.GetRequest{Cid: putResponse.Cid})
	require.NotNil(t, err)
}
