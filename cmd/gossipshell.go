package cmd

import (
	"github.com/quorumcontrol/qc3/wallet/walletshell"
	"github.com/spf13/cobra"
)

var gossipShellName string

// shellCmd represents the shell command
var gossipShellCmd = &cobra.Command{
	Use:   "gossip-shell",
	Short: "launches a shell wallet that can only talk to the testnet",
	Long:  `Do not use this for anything real as it will use hard coded signing keys for the nodes`,
	Run: func(cmd *cobra.Command, args []string) {
		walletshell.RunGossip(testShellName, TestNetGroup)
	},
}

func init() {
	rootCmd.AddCommand(gossipShellCmd)
	gossipShellCmd.Flags().StringVarP(&gossipShellName, "name", "n", "alice", "the name to use for the wallet")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// shellCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// shellCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
