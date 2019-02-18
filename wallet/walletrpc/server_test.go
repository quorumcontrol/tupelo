package walletrpc

import (
	"context"
	"os"
	"testing"

	"github.com/quorumcontrol/tupelo/gossip3/client"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/stretchr/testify/require"
)

func TestServerStartup(t *testing.T) {
	// just a simple sanity check to make sure
	// startups don't error
	path := ".tmp/servertest"
	os.RemoveAll(".tmp")
	os.MkdirAll(path, 0755)
	defer os.RemoveAll(".tmp")

	ng := types.NewNotaryGroup("ohhijusttesting")
	cli := client.New(ng)

	grpcServer, err := ServeInsecure(path, cli)
	require.Nil(t, err)
	web, err := ServeWebInsecure(grpcServer)
	require.Nil(t, err)
	http, err := ServeHttpInsecure(context.Background())
	require.Nil(t, err)
	web.Shutdown(context.Background())
	grpcServer.Stop()
	http.Shutdown(context.Background())

	secGrpcServer, err := ServeTLS(path, cli, "testassets/cert.pem", "testassets/key.pem")
	require.Nil(t, err)
	secWeb, err := ServeWebTLS(secGrpcServer, "testassets/cert.pem", "testassets/key.pem")
	require.Nil(t, err)
	secHttp, err := ServeHttpTLS(context.Background(), "testassets/cert.pem", "testassets/key.pem")
	secHttp.Shutdown(context.Background())
	secWeb.Shutdown(context.Background())
	secGrpcServer.Stop()
}
