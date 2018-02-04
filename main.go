package main

import(
	"github.com/tendermint/tmlibs/log"
	cmn "github.com/tendermint/tmlibs/common"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/tendermint/abci/server"
	"os"
)

func main() {
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

	// Create the application - in memory or persisted to disk
	app := consensus.NewBlockOwnerApp()

	// Start the listener
	srv, err := server.NewServer("tcp://0.0.0.0:4665", "socket", app)
	if err != nil {
		panic(err)
	}
	srv.SetLogger(logger.With("module", "abci-server"))
	if err := srv.Start(); err != nil {
		panic(err)
	}

	// Wait forever
	cmn.TrapSignal(func() {
		// Cleanup
		srv.Stop()
	})
}
