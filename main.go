package main

import(
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/quorumcontrol/qc3/network"
	"time"
	"fmt"
	"math/rand"
	"log"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

var privateKey = hexutil.MustDecode("0x35fba266ae900f13ab20e6182ecaf5a47edcd9b6cc3c00d8810052fbe24435c4")

func main() {
	var id = randSeq(32)

	key,_ := crypto.ToECDSA(privateKey)
	key2,_ := crypto.GenerateKey()

	fmt.Printf("id: %s", id)
	whisp := network.Start(key2)
	filter := network.CothorityFilter
	whisp.Subscribe(filter)

	whisp2 := network.Start(key)
	p2pFilter := network.NewP2PFilter(key)
	whisp2.Subscribe(p2pFilter)

	time.Sleep(15 * time.Second)
	err := network.Send(whisp, &whisper.MessageParams{
		TTL: 60, // 1 minute
		KeySym: filter.KeySym,
		Topic: whisper.BytesToTopic(network.CothorityTopic),
		PoW: 0.02,
		WorkTime: 10,
		Payload: []byte(fmt.Sprintf("hello test network! %s", id)),
	})
	if err != nil {
		log.Panicf("error sending: %v", err)
	}
	time.Sleep(5 * time.Second)

	log.Printf("sending message to p2p network")
	err = network.Send(whisp, &whisper.MessageParams{
		TTL: 60, // 1 minute
		//KeySym: filter.KeySym,
		Dst: crypto.ToECDSAPub(crypto.FromECDSAPub(&key.PublicKey)),
		Topic: whisper.BytesToTopic(network.CothorityTopic),
		PoW: 0.02,
		WorkTime: 10,
		Payload: []byte(fmt.Sprintf("hello p2p test network network! %s", id)),
	})
	if err != nil {
		log.Panicf("error sending: %v", err)
	}
	log.Printf("finished sending message to p2p network")
	time.Sleep(5 * time.Second)

	for {
		msgs := filter.Retrieve()
		for _,msg := range msgs {
			log.Printf("topic payload: %v\n", string(msg.Payload))
		}
		msgs = p2pFilter.Retrieve()
		for _,msg := range msgs {
			log.Printf("p2pFilter payload: %v\n", string(msg.Payload))
		}
		time.Sleep(1 * time.Second)
	}
}
