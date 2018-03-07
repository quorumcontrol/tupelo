package bls

/*
#cgo LDFLAGS: -L${SRCDIR}/../indy-crypto/libindy-crypto/target/release -lindy_crypto
#include "./bls.h"
*/
import "C"
import (
	"unsafe"
	"fmt"
)

type SignKey struct {
	value []byte
}

func NewSignKey() (*SignKey, error) {
	cSignKey :=  (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))
	_,err := C.indy_crypto_bls_sign_key_new(nil, 0, cSignKey)
	if err != nil {
		return nil, fmt.Errorf("error generating sign key: %v", err)
	}

	cBytes := C.CBytes(*new([]byte))
	var lenCBytes uintptr
	defer func() {
		C.indy_crypto_bls_sign_key_free(*cSignKey)
		C.free(cBytes)
	}()

	C.indy_crypto_bls_sign_key_as_bytes(*cSignKey, (**C.uint8_t)(cBytes), (*C.size_t)(unsafe.Pointer(&lenCBytes)))

	goBytes := C.GoBytes(cBytes, C.int(lenCBytes))

	return &SignKey{value: goBytes}, nil
}

func (sk *SignKey) Bytes() []byte {
	return sk.value
}

func (sk *SignKey) getCSignKey() (*unsafe.Pointer,error) {
	cSignKey := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))
	_,err := C.indy_crypto_bls_sign_key_from_bytes(nil, 0, cSignKey)
	if err != nil {
		return nil, fmt.Errorf("error generating sign key: %v", err)
	}

	cBytes := C.CBytes(sk.value)
	cBytesLen := len(sk.value)

	C.indy_crypto_bls_sign_key_from_bytes((*C.uint8_t)(cBytes), (C.size_t)(cBytesLen), cSignKey)

	defer func() {
		C.free(cBytes)
	}()

	return cSignKey, nil
}

func (sk *SignKey) Sign(msg []byte) ([]byte, error) {
	cMessageBytes := C.CBytes(msg)
	cMessageBytesLen := len(msg)

	cSignKey,err := sk.getCSignKey()
	if err != nil {
		return nil, fmt.Errorf("error getting C key: %v", err)
	}
	defer func() {
		C.indy_crypto_bls_sign_key_free(*cSignKey)
	}()

	cSignature := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))

	_,err = C.indy_crypto_bls_sign((*C.uint8_t)(cMessageBytes), (C.size_t)(cMessageBytesLen), *cSignKey, cSignature)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	cBytes := C.CBytes(*new([]byte))
	var lenCBytes uintptr
	defer func() {
		C.indy_crypto_bls_signature_free(*cSignature)
		C.free(cBytes)
	}()

	C.indy_crypto_bls_sign_key_as_bytes(*cSignKey, (**C.uint8_t)(cBytes), (*C.size_t)(unsafe.Pointer(&lenCBytes)))

	goBytes := C.GoBytes(cBytes, C.int(lenCBytes))
	return goBytes, nil
}

