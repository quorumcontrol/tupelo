package walletshell

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/abiosoft/ishell"
	"github.com/davecgh/go-spew/spew"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/signer"
	"github.com/quorumcontrol/qc3/wallet"
	"path/filepath"
	"strconv"
)

type networkClient struct {
	sessionKey *ecdsa.PrivateKey
	client     *network.Client
	group      *signer.Group
	wallet     wallet.Wallet
	signingKey *ecdsa.PrivateKey
}

func newNetworkClient(group *signer.Group) (*networkClient, error) {
	sessionKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("error generating session key: %v", err)
	}
	return &networkClient{
		sessionKey: sessionKey,
		client:     network.NewClient(sessionKey, []byte(group.Id), crypto.Keccak256([]byte(group.Id))),
		group:      group,
	}, nil
}

func (nc *networkClient) Stop() {
	nc.client.Stop()
	nc.client = network.NewClient(nc.sessionKey, []byte(nc.group.Id), crypto.Keccak256([]byte(nc.group.Id)))
}

func Run(name string, group *signer.Group) {
	// by default, new shell includes 'exit', 'help' and 'clear' commands.
	shell := ishell.New()

	// display welcome info.
	shell.Printf("Running a wallet for: %v\n", name)

	pathToWallet := filepath.Join(".storage", name+"-wallet")

	currentClient, err := newNetworkClient(group)
	if err != nil {
		panic(fmt.Sprintf("error creating network client: %v", err))
	}

	shell.AddCmd(&ishell.Cmd{
		Name: "unlock",
		Help: "unlock the current wallet",
		Func: func(c *ishell.Context) {
			c.Print("Passphrase: ")
			passphrase := c.ReadPassword()
			currentClient.wallet = wallet.NewFileWallet(passphrase, pathToWallet)
			c.Println("unlocked wallet at: ", pathToWallet)
			c.Println("starting client")
			if currentClient != nil {
				currentClient.client.Stop()
			}
			currentClient.client.Start()
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "start",
		Help: "starts the client",
		Func: func(c *ishell.Context) {
			currentClient.Stop()
			currentClient.client.Start()
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
			key, err := currentClient.wallet.GenerateKey()
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
			keys, err := currentClient.wallet.ListKeys()
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
		Name: "set-signing-key",
		Help: "set the current signing key",
		Func: func(c *ishell.Context) {
			key, err := currentClient.wallet.GetKey(c.Args[0])
			if err != nil {
				c.Println("error getting key", err)
				return
			}
			currentClient.signingKey = key
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "print-chain",
		Help: "set the current identity to a key address",
		Func: func(c *ishell.Context) {
			chain, err := currentClient.wallet.GetChain(c.Args[0])
			if err != nil {
				c.Println("error getting key", err)
				return
			}
			c.Println(spew.Sdump(chain))
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "create-chain",
		Help: "create a new chain based on a key",
		Func: func(c *ishell.Context) {
			key, err := currentClient.wallet.GetKey(c.Args[0])
			if err != nil {
				c.Println("error getting key", err)
				return
			}
			chain, err := consensus.NewSignedChainTree(key.PublicKey)
			if err != nil {
				c.Printf("error generating chain: %v", err)
				return
			}
			currentClient.wallet.SaveChain(chain)
			c.Printf("chain: %v", chain)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "list-chains",
		Help: "list the current chains",
		Func: func(c *ishell.Context) {
			ids, _ := currentClient.wallet.GetChainIds()
			for i, id := range ids {
				c.Println(strconv.Itoa(i) + ": " + id)
			}
		},
	})
	//
	//shell.AddCmd(&ishell.Cmd{
	//	Name: "get-tip",
	//	Help: "gets the tip (as known by the notary group) for a chain id",
	//	Func: func(c *ishell.Context) {
	//		tipChan, err := currentClient.GetTip(c.Args[0])
	//		if err != nil {
	//			c.Printf("error getting: %v", err)
	//			return
	//		}
	//		tip := <-tipChan
	//		c.Printf("tip: %v", tip)
	//	},
	//})
	//
	//shell.AddCmd(&ishell.Cmd{
	//	Name: "balances",
	//	Help: "gets the various balances for a chain id",
	//	Func: func(c *ishell.Context) {
	//		bals, err := currentWallet.Balances(c.Args[0])
	//		if err != nil {
	//			c.Printf("error getting balance: %v", err)
	//			return
	//		}
	//		if len(bals) == 0 {
	//			c.Println("no balances")
	//		} else {
	//			for name, bal := range bals {
	//				c.Printf("%s: %d\n", name, bal)
	//			}
	//		}
	//	},
	//})
	//
	//shell.AddCmd(&ishell.Cmd{
	//	Name: "send-coin",
	//	Help: "send coin to a destination chain",
	//	Func: func(c *ishell.Context) {
	//		c.Print("From chain: ")
	//		from := c.ReadLine()
	//		c.Print("Coin name: ")
	//		name := c.ReadLine()
	//		c.Print("Destination: ")
	//		dest := c.ReadLine()
	//		c.Print("Amount: ")
	//		amountStr := c.ReadLine()
	//		amount, _ := strconv.Atoi(amountStr)
	//
	//		c.ProgressBar().Indeterminate(true)
	//		c.ProgressBar().Start()
	//
	//		err := currentClient.SendCoin(from, dest, name, amount)
	//		if err != nil {
	//			c.Printf("error sending: %v", err)
	//		}
	//		c.ProgressBar().Stop()
	//	},
	//})
	//
	//shell.AddCmd(&ishell.Cmd{
	//	Name: "mint-coin",
	//	Help: "send coin to a destination chain",
	//	Func: func(c *ishell.Context) {
	//		c.Print("From chain: ")
	//		from := c.ReadLine()
	//		c.Print("Coin name: ")
	//		name := c.ReadLine()
	//		c.Print("Amount: ")
	//		amountStr := c.ReadLine()
	//		amount, _ := strconv.Atoi(amountStr)
	//
	//		c.ProgressBar().Indeterminate(true)
	//		c.ProgressBar().Start()
	//
	//		done, err := currentClient.MintCoin(from, name, amount)
	//		if err != nil {
	//			c.Printf("error sending: %v", err)
	//			return
	//		}
	//		<-done
	//		c.ProgressBar().Stop()
	//	},
	//})
	//
	//shell.AddCmd(&ishell.Cmd{
	//	Name: "process-messages",
	//	Help: "get the messages and process chat and receive messages",
	//	Func: func(c *ishell.Context) {
	//		c.Print("chain: ")
	//		chainId := c.ReadLine()
	//
	//		c.ProgressBar().Indeterminate(true)
	//
	//		c.ProgressBar().Start()
	//
	//		tipChan, err := currentClient.GetTip(chainId)
	//		if err != nil {
	//			c.Printf("error getting: %v", err)
	//			return
	//		}
	//		tip := <-tipChan
	//		c.ProgressBar().Stop()
	//
	//		agentAddr := crypto.PubkeyToAddress(*crypto.ToECDSAPub(tip.Agent))
	//
	//		c.Println("agentAddr: ", agentAddr.String())
	//
	//		key, err := currentWallet.GetKey(agentAddr.String())
	//		if err != nil || key == nil {
	//			c.Printf("error getting key for that chain: %v", err)
	//			return
	//		}
	//
	//		c.ProgressBar().Start()
	//
	//		currentClient.SetCurrentIdentity(key)
	//
	//		messageChan, err := currentClient.GetMessages(key)
	//		if err != nil {
	//			c.Printf("error getting key for that chain: %v", err)
	//			return
	//		}
	//
	//		time.Sleep(5 * time.Second)
	//
	//		for len(messageChan) > 0 {
	//			any := (<-messageChan).(*types.Any)
	//			obj, err := consensus.AnyToObj(any)
	//			if err != nil {
	//				c.Printf("error converting any: %v", err)
	//				return
	//			}
	//			switch any.TypeUrl {
	//			case proto.MessageName(&consensuspb.SendCoinMessage{}):
	//				c.Println("You got coin! Processing send coin message to receive the coin")
	//				done, err := currentClient.ProcessSendCoinMessage(obj.(*consensuspb.SendCoinMessage))
	//				if err != nil {
	//					c.Printf("error processing send coin: %v", err)
	//					return
	//				}
	//				<-done
	//			case proto.MessageName(&mailserverpb.ChatMessage{}):
	//				c.Println("you got mail")
	//				c.Println(string(obj.(*mailserverpb.ChatMessage).Message))
	//			default:
	//				c.Printf("unknown message type received", "typeUrl", any.TypeUrl)
	//			}
	//		}
	//
	//		c.ProgressBar().Stop()
	//		c.Println("messages received")
	//	},
	//})

	// run shell
	shell.Run()
}
