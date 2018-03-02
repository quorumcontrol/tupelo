package main

import(
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/quorumcontrol/qc3/network"
	"time"
	"fmt"
	"math/rand"
	"log"
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



func main() {
	var id = randSeq(32)

	fmt.Printf("id: %s", id)
	whisp,filter := network.Start()
	time.Sleep(30 * time.Second)
	err := network.Send(whisp, &whisper.MessageParams{
		TTL: 60 * 60 * 24, // 1 day
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

	for {
		msgs := filter.Retrieve()
		for _,msg := range msgs {
			fmt.Printf("payload: %v\n", string(msg.Payload))
		}
		time.Sleep(1 * time.Second)
	}
}
