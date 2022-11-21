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

func main() {
	var addr = flag.String("addr", ":8088", "127.0.0.1:8088 or :8088")
	var repo = flag.String("repo", "./repo", "repo path, default ./repo") //可自定义repo路径

	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//创建repo目录，如果错误并且不是已经存在错误，那么panic

	if err := os.Mkdir(*repo, 0755); err != nil && !os.IsExist(err) {
		panic(fmt.Errorf("failed to get temp dir: %s", err))
	}

	//设置和启动各个模块
	secret.RootPath = *repo
	localstore.InitDB(*repo)
	ipfsnode.Start(ctx, *repo)

	//启动web模块
	if err := web.Start(*addr); err != nil {
		fmt.Println(err.Error())
	}

}
