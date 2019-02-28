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
	ctx := context.Background()

	grpcServer, err := ServeInsecure(path, cli)
	require.Nil(t, err)

	web, err := ServeWebInsecure(ctx, grpcServer)
	require.Nil(t, err)

	web.Shutdown(ctx)
	grpcServer.Stop()

	secGrpcServer, err := ServeTLS(path, cli, "testassets/cert.pem", "testassets/key.pem")
	require.Nil(t, err)

	secWeb, err := ServeWebTLS(ctx, secGrpcServer, "testassets/cert.pem", "testassets/key.pem")
	require.Nil(t, err)

	secWeb.Shutdown(context.Background())
	secGrpcServer.Stop()
}
