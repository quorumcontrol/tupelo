package cmd

import (
	"time"

	protocol "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-protocol"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip2"
	"github.com/quorumcontrol/tupelo/gossip2client"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/spf13/cobra"
)

var setloglevel = &cobra.Command{
	Use:    "set-log-level",
	Short:  "sets log level on cluster",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		groupNodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
		group := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", groupNodeStore)
		testNetMembers := bootstrapMembers(bootstrapPublicKeys)
		group.CreateGenesisState(group.RoundAt(time.Now()), testNetMembers...)
		client := gossip2client.NewGossipClient(group, p2p.BootstrapNodes())

		payload := gossip2.ProtocolMessage{
			Payload: []byte(logLvlName),
		}

		encodedPayload, err := payload.MarshalMsg(nil)
		if err != nil {
			panic(err.Error())
		}

		for _, node := range testNetMembers {
			client.Send(node.DstKey.ToEcdsaPub(), protocol.ID(gossip2.LogLevelProtocol), encodedPayload, 10*time.Second)
		}
	},
}

func init() {
	rootCmd.AddCommand(setloglevel)
}
