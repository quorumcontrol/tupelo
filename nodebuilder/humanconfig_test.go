package nodebuilder

import (
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"testing"
)

func TestTomlLoading(t *testing.T) {
	bits, err := ioutil.ReadFile("example.toml")
	require.Nil(t, err)
	c, err := TomlToConfig(string(bits))
	require.Nil(t, err)
	require.Len(t, c.Signers, 2)
}

func TestFailsWithInvalidTracer(t *testing.T) {
	tomlStr := `
	tracingSystem = "not allowed"
	`
	_, err := TomlToConfig(tomlStr)
	require.NotNil(t, err)
}
func TestFailsWithInvalidKeys(t *testing.T) {
	tomlStr := `
	[PrivateKeySet]
	SignKeyHex = "0xasdf"
	DestKeyHex = "0xbad"
	`
	_, err := TomlToConfig(tomlStr)
	require.NotNil(t, err)
}
