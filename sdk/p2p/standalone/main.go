package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/quorumcontrol/tupelo/sdk/p2p"
)

// Standalone server for testing libp2p-js / libp2p-go interop

func startServer(ctx context.Context) (p2p.Node, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, errors.Wrap(err, "error generating key")
	}

	opts := []p2p.Option{
		p2p.WithKey(key),
		p2p.WithWebSockets(0),
	}

	return p2p.NewHostFromOptions(ctx, opts...)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := startServer(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("standalone node running at:")
	for _, addr := range h.Addresses() {
		fmt.Println(addr)
	}

	ps := h.GetPubSub()
	i := 0
	for {
		msg := fmt.Sprintf("hi - %d", i)
		err := ps.Publish("test", []byte(msg))
		if err != nil {
			panic(errors.Wrap(err, "error publishing"))
		}
		fmt.Println(msg)
		i++
		time.Sleep(2 * time.Second)
	}
}
