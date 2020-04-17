// +build wasm

package main

import (
	"context"
	"fmt"
	"syscall/js"

	logging "github.com/ipfs/go-log"

	"github.com/quorumcontrol/tupelo/sdk/wasm/jsclient"
	"github.com/quorumcontrol/tupelo/sdk/wasm/jslibs"
	"github.com/quorumcontrol/tupelo/sdk/wasm/jspubsub"
	"github.com/quorumcontrol/tupelo/sdk/wasm/jsstore"
	"github.com/quorumcontrol/tupelo/sdk/wasm/then"
)

var exitChan chan bool

var clientSingleton *jsclient.JSClient

func init() {
	exitChan = make(chan bool)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	js.Global().Get("Go").Set("exit", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		exitChan <- true
		return nil
	}))

	js.Global().Set(
		"populateLibrary",
		js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) != 2 || !args[0].Truthy() || !args[1].Truthy() {
				err := fmt.Errorf("error, must supply a valid object")
				panic(err)
			}

			helperLibs := args[1]
			cids := helperLibs.Get("cids")
			ipfsBlock := helperLibs.Get("ipfs-block")
			if !cids.Truthy() || !ipfsBlock.Truthy() {
				err := fmt.Errorf("error, must supply a library object containing cids and ipfs-block")
				go fmt.Println(err)
				panic(err)
			}
			jslibs.Cids = cids
			jslibs.IpfsBlock = ipfsBlock

			jsObj := args[0]

			jsObj.Set("generateKey", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return jsclient.GenerateKey()
			}))

			jsObj.Set("passPhraseKey", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return jsclient.PassPhraseKey(args[0], args[1])
			}))

			jsObj.Set("keyFromPrivateBytes", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return jsclient.KeyFromPrivateBytes(args[0])
			}))

			jsObj.Set("ecdsaPubkeyToDid", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return jsclient.EcdsaPubkeyToDid(args[0])
			}))

			jsObj.Set("ecdsaPubkeyToAddress", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return jsclient.EcdsaPubkeyToAddress(args[0])
			}))

			jsObj.Set("newEmptyTree", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return jsclient.NewEmptyTree(args[0], args[1])
			}))

			jsObj.Set("newNamedTree", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return clientSingleton.NewNamedTree(args[0], args[1], args[2])
			}))

			jsObj.Set("getTip", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return clientSingleton.GetTip(args[0])
			}))

			jsObj.Set("getLatest", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return clientSingleton.GetLatest(args[0])
			}))

			jsObj.Set("tokenPayloadForTransaction", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				jsOpts := args[0]
				return jsclient.TokenPayloadForTransaction(
					jsOpts.Get("blockService"),
					jsOpts.Get("tip"),
					jsOpts.Get("tokenName"),
					jsOpts.Get("sendId"),
					jsOpts.Get("jsSendTxProof"),
				)
			}))

			jsObj.Set("verifyProof", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return clientSingleton.VerifyProof(args[0])
			}))

			jsObj.Set("setLogLevel", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				logging.SetLogLevel(args[0].String(), args[1].String())
				return nil
			}))

			jsObj.Set("startClient", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				t := then.New()
				go func() {
					// js passes in:
					// interface IClientOptions {
					//     pubsub: IPubSub,
					//     notaryGroup: Uint8Array // protobuf encoded config.NotaryGroup
					//     blockService: IBlockService,
					// }
					jsOpts := args[0]

					config, err := jsclient.JsConfigToHumanConfig(jsOpts.Get("notaryGroup"))
					if err != nil {
						t.Reject(fmt.Errorf("error converting config %w", err))
						return
					}

					store := jsstore.New(jsOpts.Get("blockService"))
					bridge := jspubsub.NewPubSubBridge(jsOpts.Get("pubsub"))
					cli := jsclient.New(bridge, config, store)
					err = cli.Start(ctx)
					if err != nil {
						t.Reject(err)
						return
					}
					clientSingleton = cli
					t.Resolve(true)
				}()

				return t
			}))

			jsObj.Set("waitForRound", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				return clientSingleton.WaitForRound()
			}))

			jsObj.Set("playTransactions", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				// js passes in:
				// interface IPlayTransactionOptions {
				//     privateKey: Uint8Array,
				//     tip: CID,
				//     transactions: Uint8Array[], // protobuf encoded array of transactions.Transaction
				// }
				jsOpts := args[0]

				if clientSingleton == nil {
					t := then.New()
					t.Reject(fmt.Errorf("no client has been started"))
					return t
				}

				return clientSingleton.PlayTransactions(jsOpts.Get("privateKey"), jsOpts.Get("tip"), jsOpts.Get("transactions"))
			}))

			jsObj.Set("newAddBlockRequest", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				// js passes in:
				// interface IPlayTransactionOptions {
				//     privateKey: Uint8Array,
				//     tip: CID,
				//     transactions: Uint8Array[], // protobuf encoded array of transactions.Transaction
				// }
				jsOpts := args[0]

				if clientSingleton == nil {
					t := then.New()
					t.Reject(fmt.Errorf("no client has been started"))
					return t
				}

				return clientSingleton.NewAddBlockRequest(jsOpts.Get("privateKey"), jsOpts.Get("tip"), jsOpts.Get("transactions"))
			}))

			jsObj.Set("sendAddBlockRequest", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				// js passes in:
				// interface ISendAddBlockRequestOptions {
				//     addBlockRequest: Uint8Array,
				//     timeout?: number,
				// }
				jsOpts := args[0]

				if clientSingleton == nil {
					t := then.New()
					t.Reject(fmt.Errorf("no client has been started"))
					return t
				}

				return clientSingleton.Send(jsOpts.Get("addBlockRequest"), jsOpts.Get("timeout"))
			}))

			return jsObj
		}),
	)

	go js.Global().Get("Go").Call("readyResolver")

	<-exitChan
}
