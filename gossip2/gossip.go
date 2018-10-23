package gossip2

import (
	"crypto/ecdsa"
	"encoding/binary"

	"github.com/ipfs/go-ipld-cbor"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/qc3/p2p"
)

func init() {
	cbornode.RegisterCborType(WantMessage{})
	cbornode.RegisterCborType(ProvideMessage{})
	cbornode.RegisterCborType(ibf.InvertibleBloomFilter{})
	cbornode.RegisterCborType(ibf.DifferenceStrata{})
}

const syncProtocol = "tupelo-gossip/v1"

type IBFMap map[int]*ibf.InvertibleBloomFilter

var standardIBFSizes = []int{2000, 20000}

type GossipNode struct {
	Key     *ecdsa.PrivateKey
	Host    *p2p.Host
	Storage *BadgerStorage
	Strata  *ibf.DifferenceStrata
	IBFs    IBFMap
}

func NewGossipNode(key *ecdsa.PrivateKey, host *p2p.Host, storage *BadgerStorage) *GossipNode {
	node := &GossipNode{
		Key:     key,
		Host:    host,
		Storage: storage,
		Strata:  ibf.NewDifferenceStrata(),
		IBFs:    make(IBFMap),
	}
	for _, size := range standardIBFSizes {
		node.IBFs[size] = ibf.NewInvertibleBloomFilter(size, 4)
	}
	host.SetStreamHandler(syncProtocol, node.HandleSync)
	return node
}

func (gn *GossipNode) Add(key, value []byte) {
	ibfObjectID := byteToIBFsObjectId(key)
	err := gn.Storage.Set(key, value)
	if err != nil {
		panic("storage failed")
	}
	gn.Strata.Add(ibfObjectID)
	for _, filter := range gn.IBFs {
		filter.Add(ibfObjectID)
	}
}

func (gn *GossipNode) Remove(key []byte) {
	ibfObjectID := byteToIBFsObjectId(key)
	err := gn.Storage.Delete(key)
	if err != nil {
		panic("storage failed")
	}
	gn.Strata.Remove(ibfObjectID)
	for _, filter := range gn.IBFs {
		filter.Remove(ibfObjectID)
	}
}

func (gn *GossipNode) HandleSync(s net.Stream) {

}

func byteToIBFsObjectId(byteID []byte) ibf.ObjectId {
	return ibf.ObjectId(binary.BigEndian.Uint64(byteID))
}
