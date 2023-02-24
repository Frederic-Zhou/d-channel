package main

import (
	"d-channel/httpapi"
	"flag"
	"fmt"
	"os"
)

func main() {

	// 定义命令行参数
	port := flag.String("p", "", "The port to listen on.")

	// 解析命令行参数
	flag.Parse()

	// 如果命令行参数不存在，则从环境变量中读取
	if *port == "" {
		*port = os.Getenv("DAPPPORT")
	}

	// 如果还是不存在，则使用默认值
	if *port == "" {
		*port = "8000"
	}

	// 输出结果
	fmt.Printf("Listening on port %s...\n", *port)

	// 程序继续执行...

	httpapi.Run(fmt.Sprintf(":%s", *port))

}
