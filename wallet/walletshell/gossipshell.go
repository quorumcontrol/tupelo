package walletshell

import (
	"path/filepath"
	"strconv"

	"github.com/abiosoft/ishell"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossipclient"
	"github.com/quorumcontrol/qc3/wallet"
)

func RunGossip(name string, group *consensus.Group) {
	// by default, new shell includes 'exit', 'help' and 'clear' commands.
	shell := ishell.New()

	// display welcome info.
	shell.Printf("Running a wallet for: %v\n", name)

	pathToWallet := filepath.Join(".storage", name+"-wallet")

	currentClient := gossipclient.NewGossipClient(group)
	var currentWallet *wallet.FileWallet

	shell.AddCmd(&ishell.Cmd{
		Name: "unlock",
		Help: "unlock the current wallet",
		Func: func(c *ishell.Context) {
			c.Print("Passphrase: ")
			passphrase := c.ReadPassword()
			currentWallet = wallet.NewFileWallet(passphrase, pathToWallet)
			c.Println("unlocked wallet at: ", pathToWallet)
			c.Println("starting client")
			if currentClient != nil {
				currentClient.Stop()
				currentClient = gossipclient.NewGossipClient(group)
			}
			currentClient.Start()
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "start",
		Help: "starts the client",
		Func: func(c *ishell.Context) {
			currentClient.Stop()
			currentClient.Start()
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "stop",
		Help: "stops the client",
		Func: func(c *ishell.Context) {
			currentClient.Stop()
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
			for i, addr := range keys {
				c.Println(strconv.Itoa(i) + ": " + addr)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "create-chain",
		Help: "create a new chain based on a key",
		Func: func(c *ishell.Context) {
			key, err := currentWallet.GetKey(c.Args[0])
			if err != nil {
				c.Println("error getting key", err)
				return
			}
			chain, err := consensus.NewSignedChainTree(key.PublicKey)
			if err != nil {
				c.Printf("error generating chain: %v", err)
				return
			}
			currentWallet.SaveChain(chain)
			c.Printf("chain: %v", chain)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "list-chains",
		Help: "list the current chains",
		Func: func(c *ishell.Context) {
			ids, _ := currentWallet.GetChainIds()
			for i, id := range ids {
				c.Println(strconv.Itoa(i) + ": " + id)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "print-chain",
		Help: "set the current identity to a key address",
		Func: func(c *ishell.Context) {
			chain, err := currentWallet.GetChain(c.Args[0])
			if err != nil {
				c.Println("error getting chain", err)
				return
			}
			c.Println(chain.ChainTree.Dag.Dump())
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "get-tip",
		Help: "gets the tip (as known by the notary group) for a chain id",
		Func: func(c *ishell.Context) {
			tipResp, err := currentClient.TipRequest(c.Args[0])
			if err != nil {
				c.Printf("error getting: %v", err)
				return
			}
			if tipResp != nil {
				c.Printf("tip: %v", tipResp.Tip)
			} else {
				c.Printf("err: %v", err)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "set-data",
		Help: "set-data on a chain-tree usage: set-data chain-id key-id path value",
		Func: func(c *ishell.Context) {
			chain, err := currentWallet.GetChain(c.Args[0])
			if err != nil {
				c.Println("error getting chain", err)
				return
			}

			key, err := currentWallet.GetKey(c.Args[1])
			if err != nil {
				c.Println("error getting key", err)
				return
			}

			var remoteTip string
			if !chain.IsGenesis() {
				remoteTip = chain.Tip().String()
			}

			resp, err := currentClient.PlayTransactions(chain, key, remoteTip, []*chaintree.Transaction{
				{
					Type: consensus.TransactionTypeSetData,
					Payload: consensus.SetDataPayload{
						Path:  c.Args[2],
						Value: c.Args[3],
					},
				},
			})
			if err != nil {
				c.Printf("error playing transaction: %v", err)
			} else {
				c.Printf("resp: %v", resp)
			}

			currentWallet.SaveChain(chain)

		},
	})

	// run shell
	shell.Run()
}
