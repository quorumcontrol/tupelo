package storage

import (
	datastore "github.com/ipfs/go-datastore"
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/scrypt"
)

// EncryptedStorage is the encrypted twin of Storage
// it supports unlocking the storage with a passphrase
type EncryptedStorage interface {
	datastore.Datastore
	// Unlock takes a passphrase to use to decrypt values in the database
	Unlock(passphrase string)
}

// compile time assertion that EncryptedStore implements
// EncryptedStorage interface
var _ EncryptedStorage = (*EncryptedStore)(nil)

var saltKey = datastore.NewKey("_qcstorage_encrypted_badger_salt")

// EncryptedStore is an implementation of the EncryptedStorage interface
// using any underlying type that implements Storage interface
type EncryptedStore struct {
	datastore.Datastore
	secretKey  *[32]byte
	isUnlocked bool
}

// NewEncryptedStore takes an underlying store
// and uses encryption on the values being set
// it implements the EncryptedStorage interface
func EncryptedWrapper(store datastore.Batching) *EncryptedStore {
	return &EncryptedStore{
		Datastore: store,
	}
}

// Unlock implements the EncryptedStorage interface
func (es *EncryptedStore) Unlock(passphrase string) {
	var key [32]byte
	copy(key[:], es.passphraseToKey(passphrase))
	es.secretKey = &key
	es.isUnlocked = true
}

func (es *EncryptedStore) Close() error {
	return es.Datastore.Close()
}

// Set implements the Storage interface
func (es *EncryptedStore) Put(key datastore.Key, value []byte) error {
	if !es.isUnlocked {
		return fmt.Errorf("you must unlock this storage before using")
	}

	encrypted, err := es.encryptedValue(value)
	if err != nil {
		return fmt.Errorf("error encrypting: %v", err)
	}
	return es.Datastore.Put(key, encrypted)
}

// Get implements the Storage interface
func (es *EncryptedStore) Get(key datastore.Key) ([]byte, error) {
	if !es.isUnlocked {
		return nil, fmt.Errorf("you must unlock this storage before using")
	}
	encryptedBytes, err := es.Datastore.Get(key)
	if err != nil {
		return nil, fmt.Errorf("error getting key (%s): %v", key, err)
	}
	if len(encryptedBytes) == 0 {
		return nil, nil
	}
	decrypted, err := es.decryptedValue(encryptedBytes)
	if err != nil {
		return nil, fmt.Errorf("error decrypting: %v", err)
	}

	return decrypted, nil
}


func (es *EncryptedStore) passphraseToKey(passphrase string) []byte {
	dk, err := scrypt.Key([]byte(passphrase), es.getSalt(), 32768, 8, 1, 32)
	if err != nil {
		panic(err)
	}
	return dk
}

func (es *EncryptedStore) encryptedValue(value []byte) ([]byte, error) {
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("error getting nonce: %v", err)
	}

	encryptedValue := secretbox.Seal(nonce[:], value, &nonce, es.secretKey)
	if encryptedValue == nil {
		return nil, fmt.Errorf("error setting")
	}
	return encryptedValue, nil
}

func (es *EncryptedStore) decryptedValue(encryptedBytes []byte) ([]byte, error) {
	var decryptNonce [24]byte
	copy(decryptNonce[:], encryptedBytes[:24])
	decrypted, ok := secretbox.Open(nil, encryptedBytes[24:], &decryptNonce, es.secretKey)
	if !ok {
		return nil, fmt.Errorf("error decrypting")
	}
	return decrypted, nil
}

func (es *EncryptedStore) getSalt() []byte {
	salt, err := es.Datastore.Get(saltKey)
	if err != nil && err != datastore.ErrNotFound {
		panic(fmt.Sprintf("error getting salt key: %v", err))
	}
	if len(salt) == 0 {
		var salt [8]byte
		if _, err := io.ReadFull(rand.Reader, salt[:]); err != nil {
			panic(err)
		}
		if err := es.Datastore.Put(saltKey, salt[:]); err != nil {
			panic(err)
		}
	}

	return salt
}
