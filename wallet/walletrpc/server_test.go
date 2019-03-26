package walletrpc

import (
	"context"
	"os"
	"testing"

	"github.com/quorumcontrol/tupelo-go-client/client"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/stretchr/testify/require"
)

func TestServerStartup(t *testing.T) {
	// just a simple sanity check to make sure
	// startups don't error
	path := ".tmp/servertest"
	err := os.RemoveAll(".tmp")
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(".tmp")

	ng := types.NewNotaryGroup("ohhijusttesting")
	cli := client.New(ng)
	ctx := context.Background()

	grpcServer, err := ServeInsecure(path, cli, 0)
	require.Nil(t, err)

	web, err := ServeWebInsecure(ctx, grpcServer)
	require.Nil(t, err)

	err = web.Shutdown(context.Background())
	require.Nil(t, err)

	grpcServer.Stop()

	secGrpcServer, err := ServeTLS(path, cli, "testassets/cert.pem", "testassets/key.pem", 0)
	require.Nil(t, err)

	secWeb, err := ServeWebTLS(ctx, secGrpcServer, "testassets/cert.pem", "testassets/key.pem")
	require.Nil(t, err)

	err = secWeb.Shutdown(context.Background())
	require.Nil(t, err)

	secGrpcServer.Stop()
}
