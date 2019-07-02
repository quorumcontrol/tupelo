package storage

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBadger(t *testing.T) {
	path := ".tmp/badger"
	err := os.RemoveAll(path)
	require.Nil(t, err)
	err = os.MkdirAll(path, 0755)
	require.Nil(t, err)
	defer os.RemoveAll(".tmp")

	store, err := NewDefaultBadger(path)
	require.Nil(t, err)
	require.NotNil(t, store)
}
