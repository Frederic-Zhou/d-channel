package main

import (
	"context"
	"d-channel/ipfsnode"
	"d-channel/localstore"
	"d-channel/web"
	"flag"
	"fmt"
)

// This package is needed so that all the preloaded plugins are loaded automatically
// ldformat "github.com/ipfs/go-ipld-format"

func main() {
	var addr = flag.String("addr", ":8088", "127.0.0.1:8088 or :8088")

	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localstore.InitDB()

	ipfsnode.Start(ctx)

	if err := web.Start(*addr); err != nil {
		fmt.Println(err.Error())
	}

}
