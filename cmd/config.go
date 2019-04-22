package cmd

import (
	"io/ioutil"
	"path/filepath"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/shibukawa/configdir"
)

func readConfJson() (confB []byte, err error) {
	for _, scope := range []configdir.ConfigType{configdir.Global, configdir.System} {
		conf := configdir.New("tupelo", filepath.Join(configNamespace, "app"))
		folders := conf.QueryFolders(scope)
		fpath := filepath.Join(folders[0].Path, "conf.json")
		confB, err = ioutil.ReadFile(fpath)
		if err == nil {
			middleware.Log.Infow("loading configuration from file", "filename", fpath)
			return
		}
	}

	return
}
