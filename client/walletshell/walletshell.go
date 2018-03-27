package walletshell

import (
	"github.com/abiosoft/ishell"
	"github.com/quorumcontrol/qc3/client/wallet"
	"path/filepath"
	"github.com/quorumcontrol/qc3/client/client"
	"github.com/ethereum/go-ethereum/crypto"
	"strconv"
	"github.com/quorumcontrol/qc3/notary"
)

func Run(name string, group *notary.Group) {
	// by default, new shell includes 'exit', 'help' and 'clear' commands.
	shell := ishell.New()

	// display welcome info.
	shell.Printf("Running a wallet for: %v\n", name)

	pathToWallet := filepath.Join(".storage", name + "-wallet")

	var currentWallet wallet.Wallet
	var currentClient *client.Client

	shell.AddCmd(&ishell.Cmd{
		Name: "unlock",
		Help: "unlock the current wallet",
		Func: func(c *ishell.Context) {
			c.Print("Passphrase: ")
			passphrase := c.ReadPassword()
			currentWallet = wallet.NewFileWallet(passphrase, pathToWallet)
			c.Println("unlocked wallet at: ", pathToWallet)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "start",
		Help: "starts the client",
		Func: func(c *ishell.Context) {
			if currentClient != nil {
				currentClient.Stop()
			}
			currentClient = client.NewClient(group, currentWallet)
			currentClient.Start()
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "stop",
		Help: "stops the client",
		Func: func(c *ishell.Context) {
			if currentClient != nil {
				currentClient.Stop()
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "create-key",
		Help: "creates a new key and saves it to the wallet",
		Func: func(c *ishell.Context) {
			key, err := currentWallet.GenerateKey()
			if err != nil {
				c.Println("error generating key", err)
				return
			}
			c.Println(crypto.PubkeyToAddress(key.PublicKey).String())
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "list-keys",
		Help: "list the keys in the wallet",
		Func: func(c *ishell.Context) {
			keys, err := currentWallet.ListKeys()
			if err != nil {
				c.Println("error generating key", err)
				return
			}
			for i,addr := range keys {
				c.Println(strconv.Itoa(i) + ": " + addr)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "set-identity",
		Help: "set the current identity to a key address",
		Func: func(c *ishell.Context) {
			key,err := currentWallet.GetKey(c.Args[0])
			if err != nil {
				c.Println("error getting key", err)
				return
			}
			currentClient.SetCurrentIdentity(key)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "create-chain",
		Help: "create a new chain based on a key",
		Func: func(c *ishell.Context) {
			key,err := currentWallet.GetKey(c.Args[0])
			if err != nil {
				c.Println("error getting key", err)
				return
			}
			chain,err := currentClient.CreateChain(key)
			if err != nil {
				c.Printf("error generating chain: %v", err)
				return
			}
			c.Printf("chain: %v", chain)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "list-chains",
		Help: "list the current chains",
		Func: func(c *ishell.Context) {
			ids,_ := currentWallet.GetChainIds()
			for i,id := range ids {
				c.Println(strconv.Itoa(i) + ": " + id)
			}
		},

	})

	shell.AddCmd(&ishell.Cmd{
		Name: "get-tip",
		Help: "gets the tip (as known by the notary group) for a chain id",
		Func: func(c *ishell.Context) {
			tipChan,err := currentClient.GetTip(c.Args[0])
			if err != nil {
				c.Printf("error getting: %v", err)
			}
			tip := <-tipChan
			c.Printf("tip: %v", tip)
		},
	})

	// run shell
	shell.Run()
}