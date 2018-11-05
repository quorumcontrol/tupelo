package gossip2

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetIfExists(t *testing.T) {
	path := "setIfExistsTest"
	os.RemoveAll(path)
	os.MkdirAll(path, 0755)
	defer os.RemoveAll(path)
	storage := NewBadgerStorage(path)
	key := []byte("abcx")
	val := []byte("ab")
	didSet, err := storage.SetIfNotExists(key, val)
	require.Nil(t, err)
	assert.True(t, didSet)
	exists, err := storage.Exists(key)
	require.Nil(t, err)
	assert.True(t, exists)
}

func TestExists(t *testing.T) {
	path := "existsTest"
	os.RemoveAll(path)
	os.MkdirAll(path, 0755)
	defer os.RemoveAll(path)

	storage := NewBadgerStorage(path)
	key := []byte("abcx")
	err := storage.Set(key, []byte("a"))
	require.Nil(t, err)
	exists, err := storage.Exists(key)
	require.Nil(t, err)
	assert.True(t, exists)
}
