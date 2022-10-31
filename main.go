package main

import (
	"context"
)

// This package is needed so that all the preloaded plugins are loaded automatically
// ldformat "github.com/ipfs/go-ipld-format"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	secretkeys := GetAge("123")

	ipfsAPI, ipfsNode := StartNode(ctx)

	StartWeb(ipfsAPI, ipfsNode, secretkeys)

}
