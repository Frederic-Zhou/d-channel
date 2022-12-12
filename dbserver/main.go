package main

import (
	"context"
	"d-channel/ipfsnode"
	"flag"
	"log"
	"time"

	//使用orbitdb/cosmos，作为全局数据库
	//保存公共全局的数据信息
	//1. 频道信息（替代IPNS）
	//2. 用户信息，包含ID，共钥
	//3. 公共频道（替代所有pubsub）
	// 先使用 Orbit DB 实现

	orbitdb "berty.tech/go-orbit-db"
	"berty.tech/go-orbit-db/accesscontroller"
)

func startDB(ctx context.Context, dbname string) (err error) {
	dbpath := "./orbitdb"
	orbit, err := orbitdb.NewOrbitDB(
		ctx,
		ipfsnode.IpfsAPI,
		&orbitdb.NewOrbitDBOptions{
			Directory: &dbpath,
		})

	if err != nil {
		log.Printf("%+v", err)
		return
	}

	ac := &accesscontroller.CreateAccessControllerOptions{
		Access: map[string][]string{
			"write": {
				"*",
			},
		},
	}

	var db orbitdb.KeyValueStore

	db, err = orbit.KeyValue(ctx, dbname, &orbitdb.CreateDBOptions{
		AccessController: ac,
		Timeout:          time.Second * 600,
	})
	if err != nil {
		log.Printf("%+v", err)
		return
	}

	log.Println("dbaddr:", db.Address().String())
	log.Println("dbID:", db.Identity().ID)

	err = db.Load(ctx, 100)
	for {
		select {

		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			status := db.ReplicationStatus()
			log.Println(status.GetMax())

			value, err := db.Get(ctx, "test")
			log.Println(string(value), err)
		}
	}

}

func main() {

	var dbname = flag.String("dbname", "demo", "dbname, default 'demo'")
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ipfsnode.Start(ctx, "./repo")
	if err := startDB(ctx, *dbname); err != nil {
		panic(err)
	}
}
