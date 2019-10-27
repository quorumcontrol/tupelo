module github.com/quorumcontrol/tupelo

go 1.12

require (
	github.com/AsynkronIT/protoactor-go v0.0.0-20190429152931-21e2d03dcae5
	github.com/BurntSushi/toml v0.3.1
	github.com/dgraph-io/badger v1.6.0-rc1
	github.com/ethereum/go-ethereum v1.9.3
	github.com/gobuffalo/packr/v2 v2.5.1
	github.com/gogo/protobuf v1.3.1
	github.com/gorilla/mux v1.7.1
	github.com/hashicorp/go-immutable-radix v1.0.0
	github.com/hashicorp/golang-lru v0.5.3
	github.com/ipfs/go-cid v0.0.3
	github.com/ipfs/go-datastore v0.1.1
	github.com/ipfs/go-ds-badger v0.0.5
	github.com/ipfs/go-ipld-cbor v1.5.1-0.20190302174746-59d816225550
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v0.0.1
	github.com/libp2p/go-libp2p v0.4.0
	github.com/libp2p/go-libp2p-circuit v0.1.3
	github.com/libp2p/go-libp2p-connmgr v0.1.1
	github.com/libp2p/go-libp2p-core v0.2.3
	github.com/libp2p/go-libp2p-pubsub v0.1.1
	github.com/opentracing/opentracing-go v1.1.0
	github.com/prometheus/procfs v0.0.2 // indirect
	github.com/quorumcontrol/tupelo-go-sdk v0.5.10-0.20191027152447-f44e2c1d7832 // indirect
	github.com/shibukawa/configdir v0.0.0-20170330084843-e180dbdc8da0
	github.com/spf13/cobra v0.0.5
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.3.0
	go.dedis.ch/protobuf v1.0.6 // indirect
	go.uber.org/zap v1.10.0
	golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550
	google.golang.org/genproto v0.0.0-20190611190212-a7e196e89fd3 // indirect
	google.golang.org/grpc v1.21.1
)

replace github.com/libp2p/go-libp2p-pubsub v0.1.0 => github.com/quorumcontrol/go-libp2p-pubsub v0.0.4-0.20190528094025-e4e719f73e7a
