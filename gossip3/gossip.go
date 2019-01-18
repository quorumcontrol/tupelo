package gossip3

import "github.com/quorumcontrol/tupelo/gossip3/middleware"

func SetLogLevel(level string) error {
	return middleware.SetLogLevel(level)
}
