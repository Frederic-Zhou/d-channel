package main

import (
	"context"
	"d-channel/database"
	"log"
	"time"
)

func main() {

	ins, err := database.BootInstance(context.TODO(), database.DEFAULT_PATH, database.DEFAULT_PATH)

	if err != nil {
		log.Println(err.Error())
	}

	pk, err := ins.GetOwnPubKey()
	log.Println(ins.GetOwnID(), pk, err)

	peers, err := ins.IPFSCoreAPI.Swarm().Peers(context.TODO())
	if err != nil {
		log.Println(err.Error())
	}

	for i, v := range peers {
		log.Println(i, v.Address())
	}

	o, e := ins.Programs.Put(context.TODO(), "hello", []byte("world"))

	if e != nil {
		log.Println(e)
		return
	}

	log.Println(o)

	v, e := ins.Programs.Get(context.TODO(), "hello")
	if e != nil {
		log.Println(e)
		return
	}

	log.Println(string(v))

	time.Sleep(20 * time.Second)

	ins.Close()

}
