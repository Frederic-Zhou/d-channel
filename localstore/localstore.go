package localstore

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

const localstorePath = "./repo/localstore.json"

// 本地存储主要保存订阅的IPNS name和Receipient
var localstore LocalStore

func ReadStore() LocalStore {

	store, err := os.ReadFile(localstorePath)
	if os.IsNotExist(err) {
		localstore = LocalStore{}
		lsf, _ := json.Marshal(localstore)
		err = os.WriteFile(localstorePath, lsf, 0644)
		if err != nil {
			panic(fmt.Errorf("failed to read localstore"))
		}

		return localstore
	}

	if err != nil {
		panic(fmt.Errorf("failed to read localstore"))
	}

	localstore = LocalStore{}
	err = json.Unmarshal(store, &localstore)
	if err != nil {
		panic(fmt.Errorf("failed to read localstore"))
	}
	return localstore

}

var mutex sync.Mutex

func SaveStore() error {
	mutex.Lock()
	defer mutex.Unlock()
	lsf, _ := json.Marshal(localstore)
	return os.WriteFile(localstorePath, lsf, 0644)

}

func AddFollow(name, addr string) (LocalStore, error) {
	localstore.IPNSNames = append(localstore.IPNSNames, ipnsname{Name: name, Addr: addr})
	return localstore, SaveStore()
}

func AddPeer(name, recipient, peerPubKey string) (LocalStore, error) {
	localstore.Peers = append(localstore.Peers, peer{Name: name, Recipient: recipient, PeerPubKey: peerPubKey})
	return localstore, SaveStore()
}

type LocalStore struct {
	IPNSNames []ipnsname `json:"ipnsnames"`
	Peers     []peer     `json:"peers"`
}

type ipnsname struct {
	Name   string `json:"name"`
	Addr   string `json:"addr"`
	Latest string `json:"latest"`
}

type peer struct {
	Name       string `json:"name"`
	Recipient  string `json:"recipient"`
	PeerPubKey string `json:"peerpubkey"`
}
