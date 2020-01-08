package nodebuilder

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BurntSushi/toml"
)

func TestTomlLoading(t *testing.T) {
	c, err := TomlToConfig("testconfigs/basic.toml")
	require.Nil(t, err)
	assert.Len(t, c.NotaryGroupConfig.Signers, 2)
	assert.Equal(t, c.BootstrapNodes, []string{"/ip4/172.16.238.10/tcp/34001/ipfs/16Uiu2HAm3TGSEKEjagcCojSJeaT5rypaeJMKejijvYSnAjviWwV5"})
}

func TestNotaryGroupAbsPath(t *testing.T) {
	tomlBits, err := ioutil.ReadFile("testconfigs/basic.toml")
	require.Nil(t, err)

	var hc HumanConfig
	_, err = toml.Decode(string(tomlBits), &hc)
	require.Nil(t, err)

	tmpfile, err := ioutil.TempFile("", "basic-with-full-ng-abs-path.toml")
	require.Nil(t, err)
	defer os.Remove(tmpfile.Name())

	pwd, err := os.Getwd()
	require.Nil(t, err)
	hc.NotaryGroupConfig = filepath.Join(pwd, "testconfigs", hc.NotaryGroupConfig)
	hc.Gossip3NotaryGroupConfig = filepath.Join(pwd, "testconfigs", hc.Gossip3NotaryGroupConfig)

	encoder := toml.NewEncoder(tmpfile)
	err = encoder.Encode(hc)
	require.Nil(t, err)
	require.Nil(t, tmpfile.Close())

	c, err := TomlToConfig(tmpfile.Name())
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
