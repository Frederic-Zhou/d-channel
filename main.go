package main

import (
	"context"
	"d-channel/ipfsnode"
	"d-channel/localstore"
	"d-channel/secret"
	"d-channel/web"
	"flag"
	"fmt"
	"os"
)

// This package is needed so that all the preloaded plugins are loaded automatically
// ldformat "github.com/ipfs/go-ipld-format"
var RepoPath string

func main() {
	var addr = flag.String("addr", ":8088", "127.0.0.1:8088 or :8088")
	var repo = flag.String("repo", "./repo", "repo path, default ./repo")

	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := os.Mkdir(*repo, 0755)

	if err != nil && os.IsExist(err) {
		panic(fmt.Errorf("failed to get temp dir: %s", err))
	}

	secret.RootPath = *repo
	localstore.InitDB(*repo)
	ipfsnode.Start(ctx, *repo)

	if err := web.Start(*addr); err != nil {
		fmt.Println(err.Error())
	}

}
