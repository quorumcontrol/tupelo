package cmd

import (
	"fmt"

	"github.com/quorumcontrol/tupelo/signer/resources"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Get Tupelo version",
	Run: func(cmd *cobra.Command, _ []string) {
		version, err := resources.Version()
		if err != nil {
			panic(fmt.Sprintf("couldn't load version: %s", err))
		}
		fmt.Printf("Tupelo Version: %s\n", version)
	},
}
