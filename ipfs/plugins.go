package ipfs

import (
	"fmt"

	"github.com/ipfs/go-ipfs/plugin/loader"
)

var pluginsInjected bool

// InjectPlugins injects IPFS plugins.
//
// If IPFS plugins are already injected, this function does nothing.
func InjectPlugins() error {
	if pluginsInjected {
		return nil
	}

	plugins, err := loader.NewPluginLoader("")
	if err != nil {
		return fmt.Errorf("could not initialize IPFS plugin loader: %s", err)
	}
	if err = plugins.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize IPFS plugins: %s", err)
	}
	if err = plugins.Inject(); err != nil {
		return fmt.Errorf("failed to inject IPFS plugins: %s", err)
	}

	pluginsInjected = true

	return nil
}
