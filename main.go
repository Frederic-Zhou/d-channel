package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/interface-go-ipfs-core/options"
	// This package is needed so that all the preloaded plugins are loaded automatically
	// ldformat "github.com/ipfs/go-ipld-format"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ipfsAPI, ipfsNode := StartNode(ctx)

	StartWeb(ipfsAPI, ipfsNode)

}

func test() {

	flag.Parse()
	/// --- Part I: Getting a IPFS node running

	fmt.Println("-- Getting an IPFS node running -- ")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ipfsAPI, ipfsNode := StartNode(ctx)

	fmt.Printf("id:%s\n", ipfsNode.Identity.String())

	fmt.Println("IPFS node is running")

	/// --- Part II: add files group to ipsf like a directory
	f1 := files.NewBytesFile([]byte("hello from ipfs 101 in Kubo"))
	f2 := files.NewBytesFile([]byte("hello from ipfs 102 in Kubo"))

	dir := files.NewMapDirectory(map[string]files.Node{"f3": f1, "f1": f2, "f0": f1, "f5": f2})

	cid, err := ipfsAPI.Unixfs().Add(ctx, dir)
	if err != nil {
		panic(fmt.Errorf("could not add File: %s", err))
	}

	fmt.Printf("Added file to peer with CID %s\n", cid.String())

	/// --- Part III: get the dir ,walk datas

	nd, err := ipfsAPI.Unixfs().Get(ctx, cid)
	if err != nil {
		panic(fmt.Errorf("could not get File: %s", err))
	}

	files.Walk(nd, func(fpath string, nd files.Node) error {
		fmt.Printf("fpath:'%s'\n", fpath)

		f := files.ToFile(nd)
		if f == nil {
			return nil
		}
		defer f.Close()

		data, err := io.ReadAll(f)
		if err != nil {
			fmt.Println("read file err:", err.Error())
			return err
		}

		fmt.Printf("data:'%v'\n", string(data))
		return nil
	})

	/// --- Part IV: name publish to 'blog' key
	keys, err := ipfsAPI.Key().List(ctx)
	if err != nil {
		fmt.Println("key err", err)
	}

	blogKey, err := ipfsAPI.Key().Generate(ctx, "blog")
	if err != nil {
		for _, k := range keys {
			fmt.Println(k.Name(), k.Path(), k.ID())
			if k.Name() == "blog" {
				blogKey = k
			}

		}
	}

	nameAPI := ipfsAPI.Name()
	nameEntry, err := nameAPI.Publish(ctx, cid, options.Name.Key(blogKey.Name()))

	if err != nil {
		fmt.Printf("name err %s\n", err.Error())
	} else {
		fmt.Printf("name %s,value %s\n", nameEntry.Name(), nameEntry.Value())
	}

	fmt.Println("===\nAll done! You just finalized your first tutorial on how to use Kubo as a library")

	select {}
}
