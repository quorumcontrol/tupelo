// +build wasm

package main

import (
	"context"
	"fmt"
	"os"
	"syscall/js"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"

	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/sdk/wasm/helpers"
	"github.com/quorumcontrol/tupelo/sdk/wasm/jsclient"
	"github.com/quorumcontrol/tupelo/sdk/wasm/jslibs"
	"github.com/quorumcontrol/tupelo/sdk/wasm/jsstore"
	"github.com/quorumcontrol/tupelo/sdk/wasm/then"
	"github.com/quorumcontrol/tupelo/signer/gossip"
)

var logger = logging.Logger("validator")

var exitChan chan bool

// this is sent back to client when there's an error
// not sure why cid.Undef doesn't work, but it doesn't :(
var errorCid cid.Cid

func init() {
	exitChan = make(chan bool)
	cbornode.RegisterCborType(ValidationResponse{})
	sw := &safewrap.SafeWrap{}
	errorCid = sw.WrapObject("error").Cid()
}

type ValidationResponse struct {
	NewTip   cid.Cid
	NewNodes [][]byte
	Valid    bool
}

var validatorSingleton *gossip.TransactionValidator

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	path := os.Args[1]

	goObj := js.Global().Get("_goWasm").Get(path)

	goObj.Set("terminate", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		exitChan <- true
		return nil
	}))

	goObj.Set(
		"populateLibrary",
		js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			jsObj := args[0]

			// This is a temporary debugging thing useful to make sure go/js agree
			jsObj.Set("pubFromSig", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				t := then.New()
				go func() {
					hsh := helpers.JsBufferToBytes(args[0])
					sig := helpers.JsBufferToBytes(args[1])
					keyBits := helpers.JsBufferToBytes(args[2])
					signingKey, err := crypto.ToECDSA(keyBits)
					if err != nil {
						t.Reject(err)
						return
					}

					goSig, err := crypto.Sign(hsh, signingKey)
					if err != nil {
						t.Reject(err)
						return
					}
					fmt.Println("go sig: ", hexutil.Encode(goSig))

					recoveredPub, err := crypto.SigToPub(hsh, sig)
					if err != nil {
						t.Reject(err)
						return
					}
					t.Resolve(helpers.SliceToJSBuffer(crypto.FromECDSAPub(recoveredPub)))
				}()
				return t
			}))

			jsObj.Set("setupValidator", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				t := then.New()
				go func() {
					// js passes in:
					// interface WasmValidatorOptions {
					// 	notaryGroup: Uint8Array
					// 	tipGetter: TipGetter
					// 	store: IBlockService
					//  cids: CID,
					// }
					jsOpts := args[0]
					jslibs.Cids = jsOpts.Get("cids")

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

					tipGetter := jsOpts.Get("tipGetter")
					store := jsstore.New(jsOpts.Get("store"))
					getter, err := NewJsDagGetter(ctx, ng, tipGetter, store)
					if err != nil {
						t.Reject(fmt.Errorf("error creating jsDagGetter", err))
						return
					}
					ng.DagGetter = getter

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

					resp := &ValidationResponse{
						NewTip: errorCid,
						Valid:  isValid,
					}
					if isValid {
						resp.NewTip = newTip
						resp.NewNodes = nodesToByteSlices(newNodes)
					}

					sw := &safewrap.SafeWrap{}
					node := sw.WrapObject(resp)

					if sw.Err != nil {
						t.Reject(fmt.Errorf("error wrapping object: %w", sw.Err))
						return
					}
					t.Resolve(helpers.SliceToJSBuffer(node.RawData()))
				}()
				return t
			}))

			return jsObj
		}),
	)

	go goObj.Call("readyResolver")

	<-exitChan
}

func nodesToByteSlices(nodes []format.Node) [][]byte {
	byteArry := make([][]byte, len(nodes))
	for i, n := range nodes {
		byteArry[i] = n.RawData()
	}
	return byteArry
}
