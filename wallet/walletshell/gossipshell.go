package walletshell

import (
	"errors"
	"strconv"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/btcsuite/btcutil/base58"
	"github.com/ethereum/go-ethereum/crypto"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/tupelo/consensus"
	gossip3client "github.com/quorumcontrol/tupelo/gossip3/client"
	"github.com/quorumcontrol/tupelo/wallet/walletrpc"
)

func confirmPassword(c *ishell.Context) (string, error) {
	for tries := 0; tries < 3; tries++ {
		c.Print("Please enter a new passphrase: ")
		passphrase := c.ReadPassword()
		c.Print("Please confirm your passphrase by entering it again: ")
		confirmation := c.ReadPassword()

		if passphrase == confirmation {
			c.Println("Thank you for confirming your password.")
			return passphrase, nil
		} else {
			c.Println("Sorry, the passphrases you entered do not match.")
		}
	}

	c.Println("Sorry, none of the passwords have matched.")
	return "", errors.New("can't confirm password")
}

func RunGossip(name string, storagePath string, client *gossip3client.Client) {
	// by default, new shell includes 'exit', 'help' and 'clear' commands.
	shell := ishell.New()

	// display welcome info.
	shell.Printf("Loading shell for wallet: %v\n", name)

	// load the session
	session, err := walletrpc.NewSession(storagePath, name, client)
	if err != nil {
		shell.Printf("error loading shell: %v\n", err)
		return
	}

	shell.AddCmd(&ishell.Cmd{
		Name: "create-wallet",
		Help: "create the shell wallet",
		Func: func(c *ishell.Context) {
			c.Println("Creating wallet: ", name)

			passphrase, err := confirmPassword(c)
			if err != nil {
				c.Printf("Error creating wallet: %v\n", err)
				return
			} else {
				session.CreateWallet(passphrase)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "start-session",
		Help: "start a new session",
		Func: func(c *ishell.Context) {
			if session.IsStopped() {
				c.Print("Passphrase: ")
				passphrase := c.ReadPassword()

				c.Println("Starting session")
				err := session.Start(passphrase)
				if err != nil {
					c.Println("error starting session:", err)
					return
				}
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "stop-session",
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
				c.Println("error generating key:", err)
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
				c.Printf("error listing key: %v\n", err)
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
			if len(c.Args) < 1 {
				c.Println("not enough arguments passed to create-chain")
				return
			}

			chain, err := session.CreateChain(c.Args[0])
			if err != nil {
				c.Printf("error creating chain tree: %v\n", err)
				return
			}

			chainId, err := chain.Id()
			if err != nil {
				c.Printf("error fetching chain id: %v\n", err)
				return
			}

			c.Printf("chain-id: %v\n", chainId)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "set-owner",
		Help: "transfer ownership of a chain tree",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 3 {
				c.Println("not enough arguments passed to set-owner")
				return
			}

			newOwnerKeys := strings.Split(c.Args[2], ",")
			tip, err := session.SetOwner(c.Args[0], c.Args[1], newOwnerKeys)
			if err != nil {
				c.Printf("error setting owners: %v\n", err)
				return
			}

			c.Printf("new tip: %v\n", tip)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "export-chain",
		Help: "export an existing chain tree",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 1 {
				c.Println("not enough arguments passed to export-chain")
				return
			}

			chain, err := session.ExportChain(c.Args[0])
			if err != nil {
				c.Printf("error exporting chain tree: %v\n", err)
				return
			}

			encodedChain := base58.Encode(chain)
			c.Printf("serialized chain tree: %v\n", encodedChain)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "import-chain",
		Help: "import a chain tree",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 2 {
				c.Println("not enough arguments passed to import-chain")
				return
			}

			chainBytes := base58.Decode(c.Args[1])
			chain, err := session.ImportChain(c.Args[0], chainBytes)
			if err != nil {
				c.Printf("error importing chain tree: %v\n", err)
				return
			}

			c.Printf("chain id: %v\n", chain.Id)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "list-chains",
		Help: "list the current chain tree ids",
		Func: func(c *ishell.Context) {
			ids, err := session.GetChainIds()
			if err != nil {
				c.Printf("error listing chain: %v\n", err)
				return
			}
			for i, id := range ids {
				c.Println(strconv.Itoa(i) + ": " + id)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "print-chain",
		Help: "print an entire chain tree",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 1 {
				c.Println("not enough arguments passed to print-chain")
				return
			}

			chain, err := session.GetChain(c.Args[0])
			if err != nil {
				c.Printf("error getting chain: %v\n", err)
				return
			}
			c.Println(chain.ChainTree.Dag.Dump())
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "get-tip",
		Help: "gets the tip (as known by the notary group) for a chain id",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 1 {
				c.Println("not enough arguments passed to get-tip")
				return
			}

			tip, err := session.GetTip(c.Args[0])
			if err != nil {
				c.Printf("error getting tip: %v\n", err)
				return
			}

			c.Printf("tip: %v\n", tip)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "set-data",
		Help: "set-data on a chain-tree usage: set-data chain-id key-id path value",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 4 {
				c.Println("not enough arguments passed to set-data")
				return
			}

			data, err := cbornode.DumpObject(c.Args[3])
			if err != nil {
				c.Printf("error encoding input: %v\n", err)
				return
			}

			tip, err := session.SetData(c.Args[0], c.Args[1], c.Args[2], data)
			if err != nil {
				c.Printf("error setting data: %v\n", err)
				return
			}

			c.Printf("new tip: %v\n", tip)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "resolve",
		Help: "resolve the data at a chain-tree path. usage: resolve chain-id path",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 2 {
				c.Println("not enough arguments passed to resolve")
				return
			}

			path, err := consensus.DecodePath(c.Args[1])
			if err != nil {
				c.Printf("bad path: %v\n", err)
				return
			}

			data, remaining, err := session.Resolve(c.Args[0], path)
			if err != nil {
				c.Printf("error resolving data: %v\n", err)
				return
			}

			c.Printf("data: %v\nremaining path: %v\n", data, remaining)
		},
	})

	// run shell
	shell.Run()
}
