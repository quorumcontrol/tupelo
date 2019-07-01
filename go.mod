module github.com/quorumcontrol/tupelo

go 1.12

require (
	github.com/AsynkronIT/protoactor-go v0.0.0-20190429152931-21e2d03dcae5
	github.com/BurntSushi/toml v0.3.1
	github.com/abiosoft/ishell v2.0.0+incompatible
	github.com/abiosoft/readline v0.0.0-20180607040430-155bce2042db // indirect
	github.com/chzyer/logex v1.1.10 // indirect
	github.com/chzyer/test v0.0.0-20180213035817-a1ea475d72b1 // indirect
	github.com/dgraph-io/badger v1.6.0-rc1
	github.com/ethereum/go-ethereum v1.8.25
	github.com/flynn-archive/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/gobuffalo/packr/v2 v2.5.1
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.3.1
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.1
	github.com/hashicorp/go-immutable-radix v1.0.0
	github.com/hashicorp/golang-lru v0.5.1
	github.com/improbable-eng/grpc-web v0.9.0
	github.com/ipfs/go-cid v0.0.2
	github.com/ipfs/go-datastore v0.0.5
	github.com/ipfs/go-ds-badger v0.0.5
	github.com/ipfs/go-ipfs v0.0.0-20190627000000-8e5ea5f // 0.4.21 + badger fix ( https://github.com/ipfs/go-ipfs/pull/6473 )
	github.com/ipfs/go-ipld-cbor v1.5.1-0.20190302174746-59d816225550
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v0.0.1
	github.com/libp2p/go-libp2p v0.2.0
	github.com/libp2p/go-libp2p-circuit v0.1.0
	github.com/libp2p/go-libp2p-connmgr v0.1.0
	github.com/libp2p/go-libp2p-core v0.0.6
	github.com/libp2p/go-libp2p-pubsub v0.1.0
	github.com/opentracing/opentracing-go v1.1.0
	github.com/prometheus/common v0.6.0 // indirect
	github.com/quorumcontrol/chaintree v0.0.0-20190624152451-1b5147271beb2d334c262493abf9f6dd26a48227
	github.com/quorumcontrol/messages/build/go v0.0.0-20190603192428-dcb5ad7a31ca
	github.com/quorumcontrol/storage v1.1.4
	github.com/quorumcontrol/tupelo-go-sdk v0.0.0-20190701000000-637133866695898450dc44d4512714e248bc5b3f
	github.com/shibukawa/configdir v0.0.0-20170330084843-e180dbdc8da0
	github.com/spf13/cobra v0.0.5
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.3.0
	go.dedis.ch/protobuf v1.0.6 // indirect
	go.opencensus.io v0.22.0 // indirect
	go.uber.org/zap v1.10.0
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859
	google.golang.org/genproto v0.0.0-20190611190212-a7e196e89fd3 // indirect
	google.golang.org/grpc v1.21.1
)

replace github.com/libp2p/go-libp2p-pubsub v0.1.0 => github.com/quorumcontrol/go-libp2p-pubsub v0.0.4-0.20190528094025-e4e719f73e7a
