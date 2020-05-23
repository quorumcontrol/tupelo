module github.com/quorumcontrol/tupelo

go 1.14

require (
	github.com/AsynkronIT/protoactor-go v0.0.0-20190429152931-21e2d03dcae5
	github.com/BurntSushi/toml v0.3.1
	github.com/aws/aws-sdk-go v1.30.14
	github.com/dgraph-io/badger v1.6.1
	github.com/ethereum/go-ethereum v1.9.3
	github.com/gobuffalo/packr/v2 v2.5.1
	github.com/gogo/protobuf v1.3.1
	github.com/gorilla/mux v1.7.1
	github.com/hashicorp/golang-lru v0.5.4
	github.com/ipfs/go-bitswap v0.2.13
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.3
	github.com/ipfs/go-cid v0.0.5
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-ds-badger v0.2.4
	github.com/ipfs/go-ds-s3 v0.5.0
	github.com/ipfs/go-hamt-ipld v0.0.13
	github.com/ipfs/go-ipfs-blockstore v0.1.4
	github.com/ipfs/go-ipld-cbor v0.0.4
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log v1.0.4
	github.com/ipfs/go-merkledag v0.3.2
	github.com/libp2p/go-libp2p v0.8.3
	github.com/libp2p/go-libp2p-circuit v0.2.2
	github.com/libp2p/go-libp2p-connmgr v0.2.1
	github.com/libp2p/go-libp2p-core v0.5.3
	github.com/libp2p/go-libp2p-discovery v0.4.0
	github.com/libp2p/go-libp2p-kad-dht v0.7.11
	github.com/libp2p/go-libp2p-pubsub v0.2.7
	github.com/libp2p/go-libp2p-swarm v0.2.3
	github.com/libp2p/go-libp2p-transport-upgrader v0.2.0
	github.com/libp2p/go-msgio v0.0.4
	github.com/libp2p/go-tcp-transport v0.2.0
	github.com/libp2p/go-ws-transport v0.3.1
	github.com/multiformats/go-multiaddr v0.2.1
	github.com/multiformats/go-multiaddr-net v0.1.4
	github.com/multiformats/go-multihash v0.0.13
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/quorumcontrol/chaintree v1.0.2-0.20200501142445-c20387b71253
	github.com/quorumcontrol/messages/v2 v2.1.3-0.20200129115245-2bfec5177653
	github.com/shibukawa/configdir v0.0.0-20170330084843-e180dbdc8da0
	github.com/spf13/cobra v0.0.5
	github.com/spy16/parens v0.0.8
	github.com/stretchr/testify v1.5.1
	github.com/uber/jaeger-client-go v2.22.1+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible // indirect
	go.dedis.ch/kyber/v3 v3.0.12
	go.elastic.co/apm/module/apmot v1.7.2
	golang.org/x/crypto v0.0.0-20200423211502-4bdfaf469ed5
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
	gopkg.in/yaml.v2 v2.2.8
)

replace go.opencensus.io => github.com/quorumcontrol/opencensus-go v0.21.1-0.20200117203331-36d95539b227
