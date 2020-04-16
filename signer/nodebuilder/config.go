package nodebuilder

import (
	"crypto/ecdsa"

	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/quorumcontrol/tupelo/sdk/bls"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
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
	// @deprecated - I can't find anywhere using Namespace
	Namespace string

	NodeName string

	NotaryGroupConfig *types.Config
	PublicIP          string
	Port              int

	WebSocketPort         int
	SecureWebSocketDomain string
	CertificateCache      string

	PrivateKeySet  *PrivateKeySet
	BootstrapNodes []string

	TracingSystem TracingSystem // either Jaeger or Elastic
	BootstrapOnly bool

	Blockstore blockstore.Blockstore
	Datastore  datastore.Batching
}
