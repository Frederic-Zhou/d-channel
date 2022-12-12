package main

import (
	"context"
	"d-channel/ipfsnode"
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

var db orbitdb.KeyValueStore

func OpenDB(ctx context.Context, dbname string) (err error) {
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

	return
}

func GetDB() orbitdb.KeyValueStore {
	return db
}
