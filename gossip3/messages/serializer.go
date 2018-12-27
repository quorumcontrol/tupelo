package messages

import (
	"fmt"
	"reflect"

	"github.com/tinylib/msgp/msgp"
)

var encodableRegister = make(map[int8]func() msgp.Unmarshaler)
var typesToCodes = make(map[string]int8)
var internalEncodableRegisterCounter int8 = 1

func RegisterEncodable(obj interface{}) {
	typ := reflect.TypeOf(obj)
	typPtr := reflect.PtrTo(typ)
	newFunc := func() msgp.Unmarshaler {
		obj := reflect.New(typ).Interface()
		return (obj).(msgp.Unmarshaler)
	}
	encodableRegister[internalEncodableRegisterCounter] = newFunc
	typesToCodes[typPtr.String()] = internalEncodableRegisterCounter
	internalEncodableRegisterCounter++
}

func GetUnmarshaler(typeCode int8) msgp.Unmarshaler {
	obj, ok := encodableRegister[typeCode]
	if !ok {
		panic(fmt.Sprintf("could not find type: %d", typeCode))
	}
	return obj()
}

func GetTypeCode(obj interface{}) int8 {
	code, ok := typesToCodes[reflect.TypeOf(obj).String()]
	if !ok {
		panic(fmt.Sprintf("could not find code for %s", reflect.TypeOf(obj).String()))
	}
	return code
}
