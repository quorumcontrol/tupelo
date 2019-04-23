package walletrpc

import (
	"context"
	"os"
	"testing"

	"github.com/quorumcontrol/tupelo-go-client/gossip3/remote"
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

	pubSubSystem := remote.NewSimulatedPubSub()
	ng := types.NewNotaryGroup("ohhijusttesting")

	grpcServer, err := ServeInsecure(path, ng, pubSubSystem, 0)
	require.Nil(t, err)
	web, err := ServeWebInsecure(grpcServer)
	require.Nil(t, err)
	err = web.Shutdown(context.Background())
	require.Nil(t, err)
	grpcServer.Stop()

	secGrpcServer, err := ServeTLS(path, ng, pubSubSystem, "testassets/cert.pem", "testassets/key.pem", 0)
	require.Nil(t, err)
	secWeb, err := ServeWebTLS(secGrpcServer, "testassets/cert.pem", "testassets/key.pem")
	require.Nil(t, err)
	err = secWeb.Shutdown(context.Background())
	require.Nil(t, err)
	secGrpcServer.Stop()
}
