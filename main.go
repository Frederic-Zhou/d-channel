package main

import (
	"context"
	"d-channel/database"
	"log"
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

	ins.Close()

}
