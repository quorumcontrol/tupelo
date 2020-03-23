module github.com/quorumcontrol/tupelo/metrics

go 1.13

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

require (
	github.com/elastic/go-sysinfo v1.3.0 // indirect
	github.com/elastic/go-windows v1.0.1 // indirect
	github.com/ipfs/go-bitswap v0.1.9-0.20191015150653-291b2674f1f1
	github.com/ipfs/go-cid v0.0.3
	github.com/ipfs/go-datastore v0.3.1
	github.com/ipfs/go-ds-badger v0.2.0
	github.com/ipfs/go-hamt-ipld v0.0.13
	github.com/ipfs/go-ipfs-blockstore v0.1.0
	github.com/ipfs/go-ipld-cbor v0.0.3
	github.com/ipfs/go-log v0.0.1
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/procfs v0.0.11 // indirect
	github.com/quorumcontrol/chaintree v1.0.2-0.20200124091942-25ceb93627b9
	github.com/quorumcontrol/messages/v2 v2.1.3-0.20200129115245-2bfec5177653
	github.com/quorumcontrol/tupelo-go-sdk v0.6.0
	go.elastic.co/apm v1.6.0
	golang.org/x/sys v0.0.0-20200317113312-5766fd39f98d // indirect
	howett.net/plist v0.0.0-20200225050739-77e249a2e2ba // indirect
)
