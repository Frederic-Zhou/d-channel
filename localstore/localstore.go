package localstore

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

const localstorePath = "./localstore.json"

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

func AddFollowName(name, addr string) (LocalStore, error) {
	localstore.Names = append(localstore.Names, ipnsname{Name: name, Addr: addr})
	return localstore, SaveStore()
}

func AddRecipient(name, recipient string) (LocalStore, error) {
	localstore.Recipients = append(localstore.Recipients, otherrecipient{name, recipient})
	return localstore, SaveStore()
}

type LocalStore struct {
	Names      []ipnsname       `json:"names"`
	Recipients []otherrecipient `json:"recipients"`
}

type ipnsname struct {
	Name   string `json:"name"`
	Addr   string `json:"addr"`
	Latest string `json:"latest"`
}

type otherrecipient struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}
