package gossip2

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ipfs/go-ipld-cbor"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/tinylib/msgp/msgp"
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
	Key      *ecdsa.PrivateKey
	Host     *p2p.Host
	Storage  *BadgerStorage
	Strata   *ibf.DifferenceStrata
	Group    *consensus.NotaryGroup
	IBFs     IBFMap
	newObjCh chan ProvideMessage
}

func NewGossipNode(key *ecdsa.PrivateKey, host *p2p.Host, storage *BadgerStorage) *GossipNode {
	node := &GossipNode{
		Key:     key,
		Host:    host,
		Storage: storage,
		Strata:  ibf.NewDifferenceStrata(),
		IBFs:    make(IBFMap),
		//TODO: examine the 5 here?
		newObjCh: make(chan ProvideMessage, 5),
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

func (gn *GossipNode) DoSync() error {
	roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
	if err != nil {
		return fmt.Errorf("error getting peer: %v", err)
	}
	peer := roundInfo.RandomMember()
	ctx := context.Background()
	stream, err := gn.Host.NewStream(ctx, peer.DstKey.ToEcdsaPub(), syncProtocol)
	if err != nil {
		return fmt.Errorf("error opening new stream: %v", err)
	}
	writer := msgp.NewWriter(stream)
	reader := msgp.NewReader(stream)

	err = gn.IBFs[2000].EncodeMsg(writer)
	if err != nil {
		return fmt.Errorf("error writing IBF: %v", err)
	}
	fmt.Println("flushing")
	writer.Flush()
	var wants WantMessage
	err = wants.DecodeMsg(reader)
	if err != nil {
		return fmt.Errorf("error reading wants")
	}
	fmt.Printf("got the wants!: %v", wants)
	for _, key := range wants.Keys {
		key := uint64ToBytes(key)
		objs, err := gn.Storage.GetPairsByPrefix(key)
		if err != nil {
			return fmt.Errorf("error getting objects: %v", err)
		}
		for _, kv := range objs {
			provide := &ProvideMessage{
				Key:   kv.Key,
				Value: kv.Value,
			}
			provide.EncodeMsg(writer)
		}
	}
	last := &ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	writer.Flush()

	// now get the objects we need
	var isLastMessage bool
	for !isLastMessage {
		var provideMsg ProvideMessage
		provideMsg.DecodeMsg(reader)
		fmt.Printf("received a provide: %v", provideMsg)
		gn.newObjCh <- provideMsg
		isLastMessage = provideMsg.Last
	}
	fmt.Printf("sync complete")
	stream.Close()

	return nil
}

func (gn *GossipNode) HandleSync(stream net.Stream) {
	writer := msgp.NewWriter(stream)
	reader := msgp.NewReader(stream)

	var remoteIBF ibf.InvertibleBloomFilter
	err := remoteIBF.DecodeMsg(reader)
	if err != nil {
		panic(fmt.Sprintf("error: %v", err))
	}
	difference, err := gn.IBFs[2000].Subtract(&remoteIBF).Decode()
	if err != nil {
		panic(fmt.Sprintf("error getting diff: %f", err))
	}
	want := WantMessageFromDiff(difference.RightSet)
	err = want.EncodeMsg(writer)
	if err != nil {
		panic(fmt.Sprintf("error writing wants: %v", err))
	}
	writer.Flush()
	var isLastMessage bool
	for !isLastMessage {
		var provideMsg ProvideMessage
		provideMsg.DecodeMsg(reader)
		fmt.Printf("received a provide: %v", provideMsg)
		gn.newObjCh <- provideMsg
		isLastMessage = provideMsg.Last
	}

	toProvideAsWantMessage := WantMessageFromDiff(difference.LeftSet)
	for _, key := range toProvideAsWantMessage.Keys {
		key := uint64ToBytes(key)
		objs, err := gn.Storage.GetPairsByPrefix(key)
		if err != nil {
			panic(fmt.Sprintf("error getting objects: %v", err))
		}
		for _, kv := range objs {
			provide := &ProvideMessage{
				Key:   kv.Key,
				Value: kv.Value,
			}
			provide.EncodeMsg(writer)
		}
	}
	last := &ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	writer.Flush()
	stream.Close()
}

func byteToIBFsObjectId(byteID []byte) ibf.ObjectId {
	return ibf.ObjectId(binary.BigEndian.Uint64(byteID))
}

func uint64ToBytes(id uint64) []byte {
	a := make([]byte, 8)
	binary.BigEndian.PutUint64(a, id)
	return a
}
