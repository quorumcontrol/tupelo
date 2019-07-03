package nodebuilder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTomlLoading(t *testing.T) {
	c, err := TomlToConfig("testconfigs/basic.toml")
	require.Nil(t, err)
	require.Len(t, c.NotaryGroupConfig.Signers, 2)
}

func TestFailsWithInvalidTracer(t *testing.T) {
	_, err := TomlToConfig("./testconfigs/invalidtracer.toml")
	require.NotNil(t, err)
}
func TestFailsWithInvalidKeys(t *testing.T) {
	_, err := TomlToConfig("./testconfigs/invalidkeys.toml")
	require.NotNil(t, err)
}
