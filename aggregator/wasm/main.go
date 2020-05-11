// +build wasm

package main

import (
	"context"
	"fmt"
	"syscall/js"

	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	"github.com/multiformats/go-multihash"

	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/sdk/wasm/helpers"
	"github.com/quorumcontrol/tupelo/sdk/wasm/jsclient"
	"github.com/quorumcontrol/tupelo/sdk/wasm/jslibs"
	"github.com/quorumcontrol/tupelo/sdk/wasm/then"
	"github.com/quorumcontrol/tupelo/signer/gossip"
)

var logger = logging.Logger("validator")

var exitChan chan bool

func init() {
	exitChan = make(chan bool)
	cbornode.RegisterCborType(ValidationResponse{})
}

type ValidationResponse struct {
	NewTip   cid.Cid
	NewNodes []format.Node
	Valid    bool
}

var validatorSingleton *gossip.TransactionValidator

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

			jsObj.Set("setupValidator", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				t := then.New()
				go func() {
					// js passes in:
					// interface IValidatorOptions {
					//     notaryGroup: Uint8Array // protobuf encoded config.NotaryGroup
					// }
					jsOpts := args[0]

					config, err := jsclient.JsConfigToHumanConfig(jsOpts.Get("notaryGroup"))
					if err != nil {
						t.Reject(fmt.Errorf("error converting config %w", err))
						return
					}

					ngConfig, err := types.HumanConfigToConfig(config)
					if err != nil {
						t.Reject(fmt.Errorf("error decoding human config: %w", err))
						return
					}

					ng, err := ngConfig.NotaryGroup(nil)
					if err != nil {
						t.Reject(fmt.Errorf("error getting notary group from config: %w", err))
						return
					}

					validator, err := gossip.NewTransactionValidator(ctx, logger, ng, nil) // nil is the actor pid
					if err != nil {
						t.Reject(fmt.Errorf("error creating validator: %w", err))
						return
					}

					validatorSingleton = validator

					t.Resolve(true)
				}()

				return t
			}))

			jsObj.Set("validate", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				t := then.New()
				go func() {
					// js passes in a protobuf encoded ABR
					abrBits := helpers.JsBufferToBytes(args[0])
					abr := &services.AddBlockRequest{}
					err := abr.Unmarshal(abrBits)
					if err != nil {
						t.Reject(fmt.Errorf("error unmarshaling: %w", err))
						return
					}
					wrapper := &gossip.AddBlockWrapper{
						AddBlockRequest: abr,
					}
					newTip, isValid, newNodes, err := validatorSingleton.ValidateAbr(wrapper)
					if err != nil {
						t.Reject(fmt.Errorf("error valdating: %w", err))
						return
					}
					node, err := cbornode.WrapObject(&ValidationResponse{
						NewTip:   newTip,
						Valid:    isValid,
						NewNodes: newNodes,
					}, multihash.SHA2_256, -1)
					if err != nil {
						t.Reject(fmt.Errorf("error wrapping object: %w", err))
						return
					}
					t.Resolve(node.RawData())
				}()
				return t
			}))

			return jsObj
		}),
	)

	go js.Global().Get("Go").Call("readyResolver")

	<-exitChan
}
