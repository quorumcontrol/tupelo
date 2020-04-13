// +build wasm

package then

import (
	"fmt"
	"syscall/js"
)

// Then implements the javascript `Thenable` interface https://javascript.info/async-await
// so that Go can return something to js and js can `await` it
type Then struct {
	result       interface{}
	err          interface{}
	resolveCbs   []js.Value
	errorCbs     []js.Value
	jsObj        map[string]interface{}
	wrappedJsObj js.Value
}

type JSFunc struct {
	Value js.Value
}

// SafeInvoke is like Invoke but handles errors in the args (which js.ValueOf
// chokes on).
// Feel encouraged to add other types that js.ValueOf doesn't handle and that
// you'd like to handle more gracefully than panicking.
func (jsf JSFunc) SafeInvoke(args ...interface{}) js.Value {
	newArgs := make([]interface{}, len(args))

	for i, a := range args {
		switch ta := a.(type) {
		case error:
			newArgs[i] = js.Global().Get("Error").New(ta.Error())
		default:
			newArgs[i] = js.ValueOf(ta)
		}
	}

	return jsf.Value.Invoke(newArgs...)
}

func New() *Then {
	t := new(Then)
	jsObj := map[string]interface{}{
		"then": js.FuncOf(func(_this js.Value, args []js.Value) interface{} {
			switch len(args) {
			case 1:
				return t.handleJSThenCall(args[0], js.Null())
			case 2:
				return t.handleJSThenCall(args[0], args[1])
			default:
				return fmt.Errorf("error, must specify at least a resolve")
			}
		},
		)}
	wrappedObj := js.ValueOf(jsObj)

	t.jsObj = jsObj
	t.wrappedJsObj = wrappedObj
	return t
}

func (t *Then) handleJSThenCall(resolve js.Value, reject js.Value) error {
	if t.result != nil {
		go resolve.Invoke(js.ValueOf(t.result))
		return nil
	}

	if t.err != nil {
		if reject.Truthy() {
			go JSFunc{Value: reject}.SafeInvoke(t.err)
		}
		return nil
	}

	t.resolveCbs = append(t.resolveCbs, resolve)
	if reject.Truthy() {
		t.errorCbs = append(t.errorCbs, reject)
	}
	return nil
}

func (t *Then) Resolve(res interface{}) {
	t.result = res
	for _, cb := range t.resolveCbs {
		go func(cb js.Value) {
			cb.Invoke(js.ValueOf(res))
		}(cb)
	}
	t.resolveCbs = nil
}

func (t *Then) Reject(err interface{}) {
	t.err = err
	for _, cb := range t.errorCbs {
		go func(cb js.Value) {
			JSFunc{Value: cb}.SafeInvoke(err)
		}(cb)
	}
	t.errorCbs = nil
}

func (t *Then) JSValue() js.Value {
	return t.wrappedJsObj
}
