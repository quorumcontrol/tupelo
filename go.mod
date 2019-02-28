module github.com/quorumcontrol/tupelo

require (
	github.com/AsynkronIT/protoactor-go v0.0.0-20190103141422-46ce3cc7fd18
	github.com/Workiva/go-datastructures v1.0.50
	github.com/abiosoft/ishell v2.0.0+incompatible
	github.com/abiosoft/readline v0.0.0-20180607040430-155bce2042db // indirect
	github.com/ethereum/go-ethereum v1.8.23
	github.com/fatih/color v1.7.0 // indirect
	github.com/flynn-archive/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/gogo/protobuf v1.2.1
	github.com/golang/protobuf v1.2.0
	github.com/gorilla/mux v1.7.0
	github.com/hashicorp/go-immutable-radix v1.0.0
	github.com/hashicorp/golang-lru v0.5.0
	github.com/improbable-eng/grpc-web v0.9.0
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/ipfs/go-cid v0.9.0
	github.com/ipfs/go-ipld-cbor v1.5.1-0.20190117031511-5a60a3a4a7cc
	github.com/ipsn/go-ipfs v0.0.0-20190114233014-4b3ab38e3ef3
	github.com/mitchellh/go-homedir v1.1.0
	github.com/quorumcontrol/chaintree v4.0.1+incompatible
	github.com/quorumcontrol/differencedigest v0.0.3
	github.com/quorumcontrol/storage v1.1.1
	github.com/quorumcontrol/tupelo-go-client v0.0.0-20190225144318-1430232c6efe
	github.com/rs/cors v1.6.0 // indirect
	github.com/shibukawa/configdir v0.0.0-20170330084843-e180dbdc8da0
	github.com/spf13/cobra v0.0.3
	github.com/spf13/viper v1.3.1
	github.com/stretchr/testify v1.3.0
	golang.org/x/crypto v0.0.0-20190225124518-7f87c0fbb88b // indirect
	golang.org/x/lint v0.0.0-20181217174547-8f45f776aaf1 // indirect
	golang.org/x/net v0.0.0-20190213061140-3a22650c66bd
	golang.org/x/sys v0.0.0-20190225065934-cc5685c2db12 // indirect
	google.golang.org/grpc v1.18.0
)

replace github.com/quorumcontrol/tupelo-go-client => ../tupelo-go-client
