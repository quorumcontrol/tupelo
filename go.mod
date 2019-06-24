module github.com/quorumcontrol/tupelo

go 1.12

require (
	github.com/AsynkronIT/protoactor-go v0.0.0-20190429152931-21e2d03dcae5
	github.com/BurntSushi/toml v0.3.1
	github.com/abiosoft/ishell v2.0.0+incompatible
	github.com/abiosoft/readline v0.0.0-20180607040430-155bce2042db // indirect
	github.com/chzyer/logex v1.1.10 // indirect
	github.com/chzyer/test v0.0.0-20180213035817-a1ea475d72b1 // indirect
	github.com/ethereum/go-ethereum v1.8.25
	github.com/flynn-archive/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/gobuffalo/genny v0.1.1 // indirect
	github.com/gobuffalo/gogen v0.1.1 // indirect
	github.com/gobuffalo/packr/v2 v2.2.0
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.3.1
	github.com/gorilla/mux v1.7.1
	github.com/hashicorp/go-immutable-radix v1.0.0
	github.com/hashicorp/golang-lru v0.5.1
	github.com/improbable-eng/grpc-web v0.9.0
	github.com/ipfs/go-cid v0.0.1
	github.com/ipfs/go-ipfs v0.4.20
	github.com/ipfs/go-ipfs-config v0.0.2
	github.com/ipfs/go-ipfs-http-client v0.0.0-20190329134716-880cd0134a92
	github.com/ipfs/go-ipld-cbor v1.5.1-0.20190302174746-59d816225550
	github.com/ipfs/go-log v0.0.1
	github.com/jakehl/goid v1.1.0
	github.com/karrick/godirwalk v1.10.3 // indirect
	github.com/libp2p/go-libp2p v0.0.21
	github.com/libp2p/go-libp2p-circuit v0.0.4
	github.com/libp2p/go-libp2p-connmgr v0.0.3
	github.com/libp2p/go-libp2p-peer v0.1.0
	github.com/libp2p/go-libp2p-pubsub v0.0.3
	github.com/multiformats/go-multiaddr v0.0.2
	github.com/opentracing/opentracing-go v1.1.0
	github.com/quorumcontrol/chaintree v0.0.0-20190530190017-53765d7c259c
	github.com/quorumcontrol/messages/build/go v0.0.0-20190603192428-dcb5ad7a31ca
	github.com/quorumcontrol/storage v1.1.2
	github.com/quorumcontrol/tupelo-go-sdk v0.0.0-20190604010000-4541a467ee8900fa2a2a0711f126da3ee7452858
	github.com/shibukawa/configdir v0.0.0-20170330084843-e180dbdc8da0
	github.com/spf13/cobra v0.0.3
	github.com/stretchr/testify v1.3.0
	go.dedis.ch/protobuf v1.0.6 // indirect
	go.uber.org/zap v1.9.1
	golang.org/x/crypto v0.0.0-20190513172903-22d7a77e9e5f // indirect
	golang.org/x/net v0.0.0-20190509222800-a4d6f7feada5
	golang.org/x/sys v0.0.0-20190509141414-a5b02f93d862 // indirect
	golang.org/x/text v0.3.2 // indirect
	golang.org/x/tools v0.0.0-20190513233021-7d589f28aaf4 // indirect
	google.golang.org/grpc v1.20.0
)

replace github.com/libp2p/go-libp2p-pubsub v0.0.3 => github.com/quorumcontrol/go-libp2p-pubsub v0.0.0-20190515123400-58d894b144ff864d212cf4b13c42e8fdfe783aba
