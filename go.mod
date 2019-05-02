module github.com/quorumcontrol/tupelo

go 1.12

replace github.com/quorumcontrol/messages => ../messages/build/go

replace github.com/quorumcontrol/chaintree => ../chaintree

replace github.com/quorumcontrol/tupelo-go-client => ../tupelo-go-client

require (
	github.com/AsynkronIT/protoactor-go v0.0.0-20190318154652-aa1aa20c2fa0
	github.com/Workiva/go-datastructures v1.0.50
	github.com/abiosoft/ishell v2.0.0+incompatible
	github.com/abiosoft/readline v0.0.0-20180607040430-155bce2042db // indirect
	github.com/chzyer/logex v1.1.10 // indirect
	github.com/chzyer/test v0.0.0-20180213035817-a1ea475d72b1 // indirect
	github.com/ethereum/go-ethereum v1.8.25
	github.com/flynn-archive/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/gobuffalo/packr/v2 v2.2.0
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.3.1
	github.com/gorilla/mux v1.7.1
	github.com/hashicorp/go-immutable-radix v1.0.0
	github.com/hashicorp/golang-lru v0.5.1
	github.com/improbable-eng/grpc-web v0.9.0
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/ipfs/go-cid v0.0.1
	github.com/ipfs/go-ipfs v0.4.20
	github.com/ipfs/go-ipfs-config v0.0.2
	github.com/ipfs/go-ipfs-http-client v0.0.0-20190329134716-880cd0134a92
	github.com/ipfs/go-ipld-cbor v1.5.1-0.20190302174746-59d816225550
	github.com/ipfs/go-log v0.0.1
	github.com/jakehl/goid v1.1.0
	github.com/libp2p/go-libp2p-peer v0.1.0
	github.com/libp2p/go-libp2p-pubsub v0.0.1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-multiaddr v0.0.2
	github.com/opentracing/opentracing-go v1.1.0
	github.com/quorumcontrol/chaintree v0.0.0-20190426130059-9145fcfcdb66e952d8c7da1fc27d23169710c8d3
	github.com/quorumcontrol/messages v0.0.0-20190329073357-72154292315e
	github.com/quorumcontrol/storage v1.1.2
	github.com/quorumcontrol/tupelo-go-sdk v0.2.1-0.20190501192947-fe26b89becfba2b5fcd7e9d6f856c11ac7b7c7ab
	github.com/shibukawa/configdir v0.0.0-20170330084843-e180dbdc8da0
	github.com/spf13/cobra v0.0.3
	github.com/spf13/viper v1.3.1
	github.com/stretchr/testify v1.3.0
	go.dedis.ch/protobuf v1.0.6 // indirect
	go.uber.org/zap v1.9.1
	golang.org/x/crypto v0.0.0-20190426145343-a29dc8fdc734 // indirect
	golang.org/x/net v0.0.0-20190424112056-4829fb13d2c6
	golang.org/x/sys v0.0.0-20190426135247-a129542de9ae // indirect
	golang.org/x/text v0.3.2 // indirect
	golang.org/x/tools v0.0.0-20190425222832-ad9eeb80039a // indirect
	google.golang.org/grpc v1.20.0
)
