package main

import (
	"encoding/json"
	"fmt"
	"os"
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

func AddSubName(aka, name string) (LocalStore, error) {
	localstore.Names = append(localstore.Names, []string{aka, name})
	lsf, _ := json.Marshal(localstore)
	err := os.WriteFile(localstorePath, lsf, 0644)
	return localstore, err
}

func AddRecipient(aka, recipient string) (LocalStore, error) {
	localstore.Recipients = append(localstore.Recipients, []string{aka, recipient})
	lsf, _ := json.Marshal(localstore)
	err := os.WriteFile(localstorePath, lsf, 0644)
	return localstore, err
}

type LocalStore struct {
	Names      [][]string `json:"names"`
	Recipients [][]string `json:"recipients"`
}
