package cmd

// var setloglevel = &cobra.Command{
// 	Use:    "set-log-level",
// 	Short:  "sets log level on cluster",
// 	Hidden: true,
// 	Run: func(cmd *cobra.Command, args []string) {
// 		groupNodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
// 		group := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", groupNodeStore)
// 		testNetMembers := bootstrapMembers(bootstrapPublicKeys)
// 		group.CreateGenesisState(group.RoundAt(time.Now()), testNetMembers...)
// 		client := gossip2client.NewGossipClient(group, p2p.BootstrapNodes())

// 		payload := gossip2.ProtocolMessage{
// 			Payload: []byte(logLvlName),
// 		}

// 		encodedPayload, err := payload.MarshalMsg(nil)
// 		if err != nil {
// 			panic(err.Error())
// 		}

// 		for _, node := range testNetMembers {
// 			client.Send(node.DstKey.ToEcdsaPub(), protocol.ID(gossip2.LogLevelProtocol), encodedPayload, 10*time.Second)
// 		}
// 	},
// }

// func init() {
// 	rootCmd.AddCommand(setloglevel)
// 	setloglevel.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")
// }
