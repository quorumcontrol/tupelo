package nodestore

import (
	"context"
	"net"
	"testing"

	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
)

func TestNodestore(t *testing.T) {
	server := grpc.NewServer()
	n := NewNodestoreService()
	RegisterNodestoreServiceServer(server, n)

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

	nodestoreClient := NewNodestoreServiceClient(conn)

	session, err := nodestoreClient.Open(ctx, &OpenRequest{Config: &OpenRequest_Memory{Memory: &ConfigForMemory{}}})
	require.Nil(t, err)
	require.NotNil(t, session)
	defer nodestoreClient.Close(ctx, &CloseRequest{Session: session})

	putResponse, err := nodestoreClient.Put(ctx, &PutRequest{Session: session, Node: &Node{Bytes: obj.RawData()}})
	require.Nil(t, err)
	require.Equal(t, putResponse.Cid.Bytes, obj.Cid().Bytes())

	getResponse, err := nodestoreClient.Get(ctx, &GetRequest{Session: session, Cid: putResponse.Cid})
	require.Nil(t, err)
	require.Equal(t, getResponse.Node.Bytes, obj.RawData())

	deleteResponse, err := nodestoreClient.Delete(ctx, &DeleteRequest{Session: session, Cid: putResponse.Cid})
	require.Nil(t, err)
	require.Equal(t, deleteResponse.Success, true)

	_, err = nodestoreClient.Get(ctx, &GetRequest{Session: session, Cid: putResponse.Cid})
	require.NotNil(t, err)
}
