package cmd

import (
	"github.com/quorumcontrol/qc3/wallet/walletshell"
	"github.com/spf13/cobra"
)

var gossipShellName string

// shellCmd represents the shell command
var gossipShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "launches a shell wallet that can only talk to the testnet",
	Long:  `Do not use this for anything real as it will use hard coded signing keys for the nodes`,
	Run: func(cmd *cobra.Command, args []string) {
		walletshell.RunGossip(gossipShellName, TestNetGroup)
	},
}

func init() {
	rootCmd.AddCommand(gossipShellCmd)
	gossipShellCmd.Flags().StringVarP(&gossipShellName, "name", "n", "alice", "the name to use for the wallet")
}
