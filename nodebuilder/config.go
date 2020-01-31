package nodebuilder

import (
	"crypto/ecdsa"

	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	g3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
)

type PrivateKeySet struct {
	SignKey *bls.SignKey
	DestKey *ecdsa.PrivateKey
}

type TracingSystem int

const (
	NoTracing TracingSystem = iota
	JaegerTracing
	ElasticTracing
)

type Config struct {
	Namespace string

	Gossip3NotaryGroupConfig *g3types.Config
	NotaryGroupConfig        *types.Config
	StoragePath              string
	PublicIP                 string
	Port                     int

	WebsocketPort         int
	SecureWebSocketDomain string
	CertificateCache      string

	PrivateKeySet  *PrivateKeySet
	BootstrapNodes []string

	TracingSystem TracingSystem // either Jaeger or Elastic
	BootstrapOnly bool

	P2PStorage HumanStorageConfig
	Storage    HumanStorageConfig
}
