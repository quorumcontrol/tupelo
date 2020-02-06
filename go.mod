module github.com/quorumcontrol/tupelo

go 1.13

require (
	github.com/AsynkronIT/protoactor-go v0.0.0-20190429152931-21e2d03dcae5
	github.com/BurntSushi/toml v0.3.1
	github.com/aws/aws-sdk-go v1.28.4
	github.com/dgraph-io/badger v1.6.0
	github.com/ethereum/go-ethereum v1.9.3
	github.com/gobuffalo/packr/v2 v2.5.1
	github.com/gorilla/mux v1.7.1
	github.com/hashicorp/golang-lru v0.5.3
	github.com/ipfs/go-bitswap v0.1.9-0.20191015150653-291b2674f1f1
	github.com/ipfs/go-cid v0.0.3
	github.com/ipfs/go-datastore v0.3.1
	github.com/ipfs/go-ds-badger v0.2.0
	github.com/ipfs/go-ds-s3 v0.4.0
	github.com/ipfs/go-hamt-ipld v0.0.13
	github.com/ipfs/go-ipfs-blockstore v0.1.0
	github.com/ipfs/go-ipld-cbor v0.0.3
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v0.0.1
	github.com/libp2p/go-libp2p v0.4.0
	github.com/libp2p/go-libp2p-circuit v0.1.3
	github.com/libp2p/go-libp2p-connmgr v0.1.1
	github.com/libp2p/go-libp2p-core v0.2.4
	github.com/libp2p/go-libp2p-pubsub v0.1.1
	github.com/libp2p/go-msgio v0.0.4
	github.com/multiformats/go-multiaddr v0.1.1
	github.com/multiformats/go-multiaddr-net v0.1.0
	github.com/multiformats/go-multihash v0.0.8
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.8.1
	github.com/quorumcontrol/chaintree v1.0.2-0.20200124091942-25ceb93627b9
	github.com/quorumcontrol/messages v1.1.1
	github.com/quorumcontrol/messages/v2 v2.1.3-0.20200129115245-2bfec5177653
	github.com/quorumcontrol/tupelo-go-sdk v0.6.0-beta1.0.20200204150453-a139a1e85716
	github.com/shibukawa/configdir v0.0.0-20170330084843-e180dbdc8da0
	github.com/spf13/cobra v0.0.5
	github.com/stretchr/testify v1.3.0
	golang.org/x/crypto v0.0.0-20200204104054-c9f3fb736b72
)

// These replaces come from: https://github.com/ipfs/go-ipfs/issues/6795#issuecomment-571165734
replace (
	github.com/go-critic/go-critic => github.com/go-critic/go-critic v0.4.0
	github.com/golangci/errcheck => github.com/golangci/errcheck v0.0.0-20181223084120-ef45e06d44b6
	github.com/golangci/go-tools => github.com/golangci/go-tools v0.0.0-20190318060251-af6baa5dc196
	github.com/golangci/gofmt => github.com/golangci/gofmt v0.0.0-20181222123516-0b8337e80d98
	github.com/golangci/gosec => github.com/golangci/gosec v0.0.0-20190211064107-66fb7fc33547
	github.com/golangci/lint-1 => github.com/golangci/lint-1 v0.0.0-20190420132249-ee948d087217
	golang.org/x/xerrors => golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
	mvdan.cc/unparam => mvdan.cc/unparam v0.0.0-20190209190245-fbb59629db34
)
