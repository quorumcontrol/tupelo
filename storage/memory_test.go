package storage

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewMemoryStore(t *testing.T) {
	store := NewDefaultMemory()
	require.NotNil(t,store)
}