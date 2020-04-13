// +build wasm

package helpers

import (
	"syscall/js"

	"github.com/quorumcontrol/tupelo/sdk/wasm/jslibs"

	"github.com/ipfs/go-cid"
)

func JsStringArrayToStringSlice(jsStrArray js.Value) []string {
	len := jsStrArray.Length()
	strs := make([]string, len)
	for i := 0; i < len; i++ {
		strs[i] = jsStrArray.Index(i).String()
	}
	return strs
}

func JsBufferToBytes(buf js.Value) []byte {
	len := buf.Length()
	bits := make([]byte, len)
	js.CopyBytesToGo(bits, buf)
	return bits
}

func SliceToJSBuffer(slice []byte) js.Value {
	return js.Global().Get("Buffer").Call("from", SliceToJSArray(slice))
}

func JsCidToCid(jsCid js.Value) (cid.Cid, error) {
	buf := jsCid.Get("buffer")
	bits := JsBufferToBytes(buf)
	return cid.Cast(bits)
}

func CidToJSCID(c cid.Cid) js.Value {
	bits := c.Bytes()
	jsBits := SliceToJSBuffer(bits)
	return jslibs.Cids.New(jsBits)
}

func SliceToJSArray(slice []byte) js.Value {
	jsArry := js.Global().Get("Uint8Array").New(len(slice))
	js.CopyBytesToJS(jsArry, slice)
	return jsArry
}
