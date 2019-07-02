package storage

import (
	"github.com/stretchr/testify/require"
	datastore "github.com/ipfs/go-datastore"
	"testing"
)

func TestEncrypted(t *testing.T) {
	store := NewDefaultMemory()
	encrypted := EncryptedWrapper(store)
	encrypted.Unlock("test")
	k := datastore.NewKey("test")
	err := encrypted.Put(k, []byte("hi"))
	require.Nil(t,err)

	val,err := encrypted.Get(k)
	require.Nil(t,err)
	require.Equal(t, []byte("hi"), val)
}