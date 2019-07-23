package nodebuilder

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func TestTomlLoading(t *testing.T) {
	c, err := TomlToConfig("testconfigs/basic.toml")
	require.Nil(t, err)
	assert.Len(t, c.NotaryGroupConfig.Signers, 2)
	assert.Equal(t, c.BootstrapNodes, []string{"/ip4/172.16.238.10/tcp/34001/ipfs/16Uiu2HAm3TGSEKEjagcCojSJeaT5rypaeJMKejijvYSnAjviWwV5"})
}

func TestFailsWithInvalidTracer(t *testing.T) {
	_, err := TomlToConfig("./testconfigs/invalidtracer.toml")
	require.NotNil(t, err)
}
func TestFailsWithInvalidKeys(t *testing.T) {
	_, err := TomlToConfig("./testconfigs/invalidkeys.toml")
	require.NotNil(t, err)
}
