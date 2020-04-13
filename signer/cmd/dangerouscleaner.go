package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var forceDestroy bool
var namespaceToDestroy string

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "this command will wipe out wallets, keys, etc for any namespace (the whole namespace). use caution.",
	Run: func(cmd *cobra.Command, args []string) {
		if namespaceToDestroy == "" {
			fmt.Println("you must specify the namespace to destroy using the --namespace flag")
			os.Exit(1)
		}

		if !forceDestroy {
			fmt.Println("you must specify the --force flag in order to execute this dangerous operation. Selected namesapce:", namespaceToDestroy)
			os.Exit(1)
		}

		configNamespace = namespaceToDestroy

		fmt.Println("destroy namespace", namespaceToDestroy)

		// leave the configDir argument empty in order to get the parent namespace directory
		path := configDir("")
		fmt.Println("removing path", path)
		err := os.RemoveAll(path)
		if err != nil {
			panic(fmt.Errorf("error removing path: %v", err))
		}
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.Flags().BoolVar(&forceDestroy, "force", false, "make sure you really want to drop everything and then specify force here")
	destroyCmd.Flags().StringVar(&namespaceToDestroy, "namespace", "", "which namespace do you want to destroy")
}
