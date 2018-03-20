// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"github.com/spf13/cobra"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/quorumcontrol/qc3/network"
	"time"
	"fmt"
	"math/rand"
	"log"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common/hexutil"
)


var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

var privateKey = hexutil.MustDecode("0x35fba266ae900f13ab20e6182ecaf5a47edcd9b6cc3c00d8810052fbe24435c4")

// testNetworkCmd represents the testNetwork command
var testNetworkCmd = &cobra.Command{
	Use:   "test-network",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		doTestNetwork()
	},
}

func init() {
	rootCmd.AddCommand(testNetworkCmd)
	rand.Seed(time.Now().UnixNano())


	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// testNetworkCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// testNetworkCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}


func doTestNetwork() {
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
