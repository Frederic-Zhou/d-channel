package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"filippo.io/age"
)

func GetAge() (keys []key) {

	keystore, err := createKeystore()
	if err != nil {
		panic(fmt.Errorf("failed to create key store"))
	}

	filepath.Walk(keystore, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			panic(fmt.Errorf("keystore walk:%s", err.Error()))
		}

		if info.IsDir() {
			return nil
		}

		keyfile, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		key := key{Recipient: filepath.Base(path), Identity: string(keyfile)}

		keys = append(keys, key)

		return nil
	})

	if len(keys) == 0 {
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			panic(fmt.Errorf("failed to spawn age indetity: %s", err))
		}

		key := key{
			Identity:  identity.String(),
			Recipient: identity.Recipient().String(),
		}

		err = os.WriteFile(filepath.Join(keystore, key.Recipient), []byte(key.Identity), 0644)
		if err != nil {
			panic(fmt.Errorf("failed to write key store"))
		}

		keys = append(keys, key)

	}

	return keys
}

func createKeystore() (string, error) {
	keystore := "./keystore"

	err := os.Mkdir(keystore, 0755)

	if os.IsExist(err) {
		return keystore, nil
	}

	if err != nil {
		os.Remove(keystore)
		return "", err
	}

	return keystore, nil
}

type key struct {
	Identity  string
	Recipient string
}
