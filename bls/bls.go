package bls

/*
#cgo CFLAGS: -I${SRCDIR}/../indy-crypto/libindy-crypto/target/release
#cgo LDFLAGS: -L${SRCDIR}/../indy-crypto/libindy-crypto/target/release -Wl,-rpath=\$ORIGIN/indy-crypto/libindy-crypto/target/release -lindy_crypto
#include "./bls.h"
*/
import "C"
import (
	"fmt"
	"log"
	"unsafe"

	"os"
	"os/signal"
	"syscall"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

const GeneratorHex = "0x00505d3670be80403e051ea1fe991e21a65aa7a34fb217faaaeece6d07f4ace018a4598fa281ccd9604a24024146861defe23200344c20ee95780eda2c5bd3630a7bd596e91c1e8359e503c088a9eeb87a895821e2ea7d96c39fc1acc5d9453d1957e94588afaf7fc0a232d77d4f73097b4c66ec4bce715e58023031ac289b4a"

var GeneratorBytes []byte
var CGenerator *unsafe.Pointer

func init() {
	GeneratorBytes = hexutil.MustDecode(GeneratorHex)
	CGenerator = getStandardGenerator()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		C.indy_crypto_bls_generator_free(*CGenerator)
		os.Exit(0)
	}()
}

func SetupLogger() {
	C.indy_crypto_init_logger()
}

type SignKey struct {
	value []byte
}

type VerKey struct {
	value []byte
}

func BytesToSignKey(keyBytes []byte) *SignKey {
	return &SignKey{
		value: keyBytes,
	}
}

func BytesToVerKey(keyBytes []byte) *VerKey {
	return &VerKey{
		value: keyBytes,
	}
}

func NewSignKey() (*SignKey, error) {
	cSignKey := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))
	_, err := C.indy_crypto_bls_sign_key_new(nil, 0, cSignKey)
	if err != nil {
		return nil, fmt.Errorf("error generating sign key: %v", err)
	}

	cBytes := C.CBytes(make([]byte, 32))
	var cBytesLen uintptr
	defer func() {
		C.indy_crypto_bls_sign_key_free(*cSignKey)
		C.free(cBytes)
	}()

	code, err := C.indy_crypto_bls_sign_key_as_bytes(*cSignKey, (**C.uint8_t)(cBytes), (*C.size_t)(unsafe.Pointer(&cBytesLen)))
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error getting sign key as bytes: %v, code: %d", err, code)
	}

	goBytes := C.GoBytes(*(*unsafe.Pointer)(cBytes), C.int(cBytesLen))

	return &SignKey{value: goBytes}, nil
}

func MustNewSignKey() *SignKey {
	key, err := NewSignKey()
	if err != nil {
		panic(fmt.Sprintf("error generating key: %v", err))
	}
	return key
}

func (sk *SignKey) Bytes() []byte {
	return sk.value
}

func (sk *SignKey) getCSignKey() (*unsafe.Pointer, error) {
	cSignKey := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))

	cBytes := C.CBytes(sk.value)
	defer C.free(cBytes)
	cBytesLen := len(sk.value)

	code, err := C.indy_crypto_bls_sign_key_from_bytes((*C.uint8_t)(cBytes), (C.size_t)(cBytesLen), cSignKey)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error getting sign key from bytes: %v code: %d", err, code)
	}

	return cSignKey, nil
}

func (sk *SignKey) Sign(msg []byte) ([]byte, error) {
	cMessageBytes := C.CBytes(msg)
	defer C.free(cMessageBytes)
	cMessageBytesLen := len(msg)

	cSignKey, err := sk.getCSignKey()
	if err != nil {
		return nil, fmt.Errorf("error getting C key: %v", err)
	}
	defer C.indy_crypto_bls_sign_key_free(*cSignKey)

	cSignature := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))

	code, err := C.indy_crypto_bls_sign((*C.uint8_t)(cMessageBytes), (C.size_t)(cMessageBytesLen), *cSignKey, cSignature)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error signing: %v code: %d", err, code)
	}
	defer C.indy_crypto_bls_signature_free(*cSignature)

	cBytes := C.CBytes(make([]byte, 128))
	var cBytesLen uintptr
	defer C.free(cBytes)

	code, err = C.indy_crypto_bls_signature_as_bytes(*cSignature, (**C.uint8_t)(cBytes), (*C.size_t)(unsafe.Pointer(&cBytesLen)))
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error signing: %v code: %d", err, code)
	}

	goBytes := C.GoBytes(*(*unsafe.Pointer)(cBytes), C.int(cBytesLen))
	return goBytes, nil
}

func (sk *SignKey) VerKey() (*VerKey, error) {
	cVerKey := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))
	cGenerator := CGenerator

	cSignKey, err := sk.getCSignKey()
	if err != nil {
		return nil, fmt.Errorf("error getting sign key: %v", err)
	}
	defer C.indy_crypto_bls_sign_key_free(*cSignKey)

	code, err := C.indy_crypto_bls_ver_key_new(*cGenerator, *cSignKey, cVerKey)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error getting verkey: %v", err)
	}

	cBytes := C.CBytes(make([]byte, 128))
	var cBytesLen uintptr
	defer C.free(cBytes)

	code, err = C.indy_crypto_bls_ver_key_as_bytes(*cVerKey, (**C.uint8_t)(cBytes), (*C.size_t)(unsafe.Pointer(&cBytesLen)))
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error getting bytes: %v", err)
	}

	goBytes := C.GoBytes(*(*unsafe.Pointer)(cBytes), C.int(cBytesLen))

	return &VerKey{
		value: goBytes,
	}, nil
}

func (sk *SignKey) MustVerKey() *VerKey {
	verKey, err := sk.VerKey()
	if err != nil {
		log.Panicf("error getting verKey: %v", err)
	}
	return verKey
}

func (vk *VerKey) Bytes() []byte {
	return vk.value
}

func (vk *VerKey) getCVerKey() (*unsafe.Pointer, error) {
	cSignKey := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))

	cBytes := C.CBytes(vk.value)
	cBytesLen := len(vk.value)
	defer C.free(cBytes)

	code, err := C.indy_crypto_bls_ver_key_from_bytes((*C.uint8_t)(cBytes), (C.size_t)(cBytesLen), cSignKey)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error getting verkey: %v", err)
	}

	return cSignKey, nil
}

func (vk *VerKey) Verify(sig, msg []byte) (bool, error) {
	cSignature, err := cSignatureFrom(sig)
	if err != nil {
		return false, fmt.Errorf("error getting signature: %v", err)
	}
	defer C.indy_crypto_bls_signature_free(*cSignature)

	cVerKey, err := vk.getCVerKey()
	if err != nil {
		return false, fmt.Errorf("error getting cVerkey: %v", err)
	}
	defer C.indy_crypto_bls_ver_key_free(*cVerKey)

	cGenerator := CGenerator

	cMessageBytes := C.CBytes(msg)
	defer C.free(cMessageBytes)
	cMessageBytesLen := len(msg)

	isVerified := false

	code, err := C.indy_crypto_bsl_verify(*cSignature, (*C.uint8_t)(cMessageBytes), (C.size_t)(cMessageBytesLen), *cVerKey, *cGenerator, (*C.bool)(&isVerified))
	if err != nil || code != 0 {
		return false, fmt.Errorf("error verifying: %v code: %d", err, code)
	}

	return isVerified, nil
}

func SumSignatures(sigs [][]byte) ([]byte, error) {
	cSigs := make([]unsafe.Pointer, len(sigs))
	for i, sig := range sigs {
		cSig, err := cSignatureFrom(sig)
		if err != nil {
			return nil, fmt.Errorf("error getting csig from %v", sig)
		}
		cSigs[i] = *cSig
	}
	defer func() {
		for _, cSig := range cSigs {
			C.indy_crypto_bls_signature_free(cSig)
		}
	}()

	cMultiSig := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))

	code, err := C.indy_crypto_bls_multi_signature_new((*unsafe.Pointer)(unsafe.Pointer(&cSigs[0])), C.size_t(len(cSigs)), cMultiSig)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error creating multi: %v code: %d", err, code)
	}
	defer C.indy_crypto_bls_multi_signature_free(*cMultiSig)

	cBytes := C.CBytes(make([]byte, 128))
	var cBytesLen uintptr
	defer C.free(cBytes)

	code, err = C.indy_crypto_bls_multi_signature_as_bytes(*cMultiSig, (**C.uint8_t)(cBytes), (*C.size_t)(unsafe.Pointer(&cBytesLen)))
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error getting bytes: %v", err)
	}

	goBytes := C.GoBytes(*(*unsafe.Pointer)(cBytes), C.int(cBytesLen))
	return goBytes, nil
}

func VerifyMultiSig(sig, msg []byte, verKeys [][]byte) (bool, error) {
	cSignature, err := cMultiSigFrom(sig)
	if err != nil {
		return false, fmt.Errorf("error getting signature: %v", err)
	}
	defer C.indy_crypto_bls_multi_signature_free(*cSignature)

	cVerKeys := make([]unsafe.Pointer, len(verKeys))
	for i, verKeyBytes := range verKeys {
		verKey := &VerKey{value: verKeyBytes}
		cVerKey, err := verKey.getCVerKey()
		if err != nil {
			return false, fmt.Errorf("error getting verkey: %v", err)
		}
		cVerKeys[i] = *cVerKey
	}
	defer func() {
		for _, cVerKey := range cVerKeys {
			C.indy_crypto_bls_ver_key_free(cVerKey)
		}
	}()

	cGenerator := CGenerator

	cMessageBytes := C.CBytes(msg)
	defer C.free(cMessageBytes)
	cMessageBytesLen := len(msg)

	isVerified := false

	code, err := C.indy_crypto_bls_verify_multi_sig(*cSignature, (*C.uint8_t)(cMessageBytes), (C.size_t)(cMessageBytesLen), (*unsafe.Pointer)(unsafe.Pointer(&cVerKeys[0])), C.size_t(len(cVerKeys)), *cGenerator, (*C.bool)(&isVerified))
	if err != nil || code != 0 {
		return false, fmt.Errorf("error verifying: %v code: %d", err, code)
	}

	return isVerified, nil
}

func cSignatureFrom(sig []byte) (*unsafe.Pointer, error) {
	cBytes := C.CBytes(sig)
	defer C.free(cBytes)
	cBytesLen := len(sig)

	cSignature := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))

	code, err := C.indy_crypto_bls_signature_from_bytes((*C.uint8_t)(cBytes), (C.size_t)(cBytesLen), cSignature)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error getting signature: %v, code: %d", err, code)
	}
	return cSignature, nil
}

func cMultiSigFrom(sig []byte) (*unsafe.Pointer, error) {
	cBytes := C.CBytes(sig)
	defer C.free(cBytes)
	cBytesLen := len(sig)

	cSignature := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))

	code, err := C.indy_crypto_bls_multi_signature_from_bytes((*C.uint8_t)(cBytes), (C.size_t)(cBytesLen), cSignature)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error getting signature: %v, code: %d", err, code)
	}
	return cSignature, nil
}

func getStandardGenerator() *unsafe.Pointer {
	cGenerator := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))
	cBytes := C.CBytes(GeneratorBytes)
	defer C.free(cBytes)
	cBytesLen := len(GeneratorBytes)

	code, err := C.indy_crypto_bls_generator_from_bytes((*C.uint8_t)(cBytes), (C.size_t)(cBytesLen), cGenerator)
	if err != nil || code != 0 {
		log.Panicf("error getting generator: %v", err)
	}
	return cGenerator
}

// We generally don't need to call the new generator method as we've hard coded a generator
func NewGenerator() ([]byte, error) {
	cGenerator := (*unsafe.Pointer)(unsafe.Pointer(new(interface{})))

	code, err := C.indy_crypto_bls_generator_new(cGenerator)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error generating: %v", err)
	}
	cBytes := C.CBytes(make([]byte, 128))
	var cBytesLen uintptr

	defer func() {
		C.free(cBytes)
		C.indy_crypto_bls_generator_free(*cGenerator)
	}()

	code, err = C.indy_crypto_bls_generator_as_bytes(*cGenerator, (**C.uint8_t)(cBytes), (*C.size_t)(unsafe.Pointer(&cBytesLen)))
	if err != nil || code != 0 {
		return nil, fmt.Errorf("error getting bytes: %v", err)
	}

	goBytes := C.GoBytes(*(*unsafe.Pointer)(cBytes), C.int(cBytesLen))

	return goBytes, nil
}
