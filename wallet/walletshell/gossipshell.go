package walletshell

import (
	"strconv"

	"github.com/abiosoft/ishell"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/wallet/walletrpc"
)

func RunGossip(name string, group *consensus.NotaryGroup) {
	// by default, new shell includes 'exit', 'help' and 'clear' commands.
	shell := ishell.New()

	// display welcome info.
	shell.Printf("Starting shell for wallet: %v\n", name)

	// load the session
	session, err := walletrpc.NewSession(name, group)
	if err != nil {
		shell.Printf("error starting shell: %v", err)
		return
	}

	shell.AddCmd(&ishell.Cmd{
		Name: "create-wallet",
		Help: "create the shell wallet",
		Func: func(c *ishell.Context) {
			c.Print("Passphrase: ")
			passphrase := c.ReadPassword()

			c.Println("Creating wallet: ", name)
			session.CreateWallet(passphrase)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "start",
		Help: "start a new session",
		Func: func(c *ishell.Context) {
			c.Print("Passphrase: ")
			passphrase := c.ReadPassword()

			c.Println("Starting session")
			err := session.Start(passphrase)
			if err != nil {
				c.Println("error starting session:", err)
				return
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "stop",
		Help: "stops the session",
		Func: func(c *ishell.Context) {
			c.Println("Stopping session")
			session.Stop()
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "create-key",
		Help: "creates a new key and saves it to the wallet",
		Func: func(c *ishell.Context) {
			key, err := session.GenerateKey()
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
			keys, err := session.ListKeys()
			if err != nil {
				c.Printf("error listing key: %v", err)
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
			chain, err := session.CreateChain(c.Args[0])
			if err != nil {
				c.Printf("error creating chain tree: %v", err)
				return
			}

			c.Printf("chain: %v", chain)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "list-chains",
		Help: "list the current chains",
		Func: func(c *ishell.Context) {
			ids, _ := session.GetChainIds()
			for i, id := range ids {
				c.Println(strconv.Itoa(i) + ": " + id)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "print-chain",
		Help: "print an entire chain tree",
		Func: func(c *ishell.Context) {
			chain, err := session.GetChain(c.Args[0])
			if err != nil {
				c.Printf("error getting chain: %v", err)
				return
			}
			c.Println(chain.ChainTree.Dag.Dump())
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "get-tip",
		Help: "gets the tip (as known by the notary group) for a chain id",
		Func: func(c *ishell.Context) {
			tip, err := session.GetTip(c.Args[0])
			if err != nil {
				c.Printf("error getting tip: %v", err)
				return
			}

			c.Printf("tip: %v", tip)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "set-data",
		Help: "set-data on a chain-tree usage: set-data chain-id key-id path value",
		Func: func(c *ishell.Context) {
			sw := &safewrap.SafeWrap{}
			dataNode := sw.WrapObject(c.Args[3])
			data := dataNode.RawData()

			tip, err := session.SetData(c.Args[0], c.Args[1], c.Args[2], data)
			if err != nil {
				c.Printf("error setting data: %v", err)
				return
			}

			c.Printf("new tip: %v", tip)
		},
	})

	// run shell
	shell.Run()
}
