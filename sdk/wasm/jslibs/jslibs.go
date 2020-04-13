// +build wasm

package jslibs

import (
	"syscall/js"
)

// jslibs is here to hold global references to javascript helper projects
// this way wasm doesn't have to call into javascript and do a "require" as that doesn't work in the browser

// when the caller is populating their library (using the populateLibrary call on main), they pass
// in references to these helper libs and the main.go sets these for use in all the various wasm
// packages.

var IpfsBlock js.Value
var Cids js.Value
