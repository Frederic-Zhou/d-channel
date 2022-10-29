package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"filippo.io/age"
	"filippo.io/age/armor"
)

const keysFile = "./keys.json"

func GetAge() (key SecretKeys) {

	//从文件中读取key
	//如果文件不存在，创建
	//如果存在，读取到keyJson，然后通过age.Parse方法，将文本的key转换为对象返回
	keysfile, err := os.ReadFile(keysFile)
	keyJson := keyJson{}
	if os.IsNotExist(err) { //如果不存在错误，创建新key，并写入到keyJson
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			panic(fmt.Errorf("failed to spawn age indetity: %s", err))
		}

		keyJson.Identities = []string{identity.String()}
		keyJson.Recipient = identity.Recipient().String()

		data, err := json.Marshal(keyJson)
		if err != nil {
			panic(fmt.Errorf("failed to json format keys"))
		}
		err = os.WriteFile(keysFile, data, 0644)
		if err != nil {
			panic(fmt.Errorf("failed to write key store"))
		}

	} else if err != nil { //如果是非不存在的其他错误
		panic(fmt.Errorf("failed to read %s", err.Error()))
	} else { //存在，并且没有错误，用文件转换为keyJson
		err = json.Unmarshal(keysfile, &keyJson)
		if err != nil {
			panic(fmt.Errorf("failed to Unmarshal %s", err.Error()))
		}
	}

	key.Recipient, err = age.ParseX25519Recipient(keyJson.Recipient)
	if err != nil {
		panic(fmt.Errorf("failed to read key Recipient %s", err.Error()))
	}

	for _, s := range keyJson.Identities {
		id, err := age.ParseX25519Identity(s)
		if err != nil {
			panic(fmt.Errorf("failed to read identities %s", err.Error()))
		}
		key.Identities = append(key.Identities, id)
	}

	return

}

type SecretKeys struct {
	Identities []age.Identity `json:"identities"`
	Recipient  age.Recipient  `json:"recipient"`
}
type keyJson struct {
	Identities []string `json:"identities"`
	Recipient  string   `json:"recipient"`
}

func Encrypt(recipients []age.Recipient, in io.Reader, out io.Writer) error {

	a := armor.NewWriter(out)
	defer func() {
		if err := a.Close(); err != nil {
			return
		}
	}()
	out = a

	w, err := age.Encrypt(out, recipients...)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, in); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return nil
}

func Decrypt(identities []age.Identity, in io.Reader, out io.Writer) error {
	rr := bufio.NewReader(in)
	var r io.Reader
	//如果不是amor开头，认为不是加密文件，返回原始数据到out
	if start, _ := rr.Peek(len(armor.Header)); string(start) == armor.Header {
		in = armor.NewReader(rr)
		var err error
		r, err = age.Decrypt(in, identities...)
		if err != nil {
			return err
		}
	} else {
		r = rr
	}

	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	return nil
}

func NewSecretKey() (key SecretKeys, err error) {
	//从文件读取
	keysfile, err := os.ReadFile(keysFile)
	if err != nil {
		return
	}
	keyJson := keyJson{}

	//转换为keyJson
	err = json.Unmarshal(keysfile, &keyJson)
	if err != nil {
		panic(fmt.Errorf("failed to Unmarshal %s", err.Error()))
	}

	//生成新的
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		panic(fmt.Errorf("failed to spawn age indetity: %s", err))
	}

	//保存到keyJson中，并从新写入到keyfile
	keyJson.Identities = append(keyJson.Identities, identity.String())
	keyJson.Recipient = identity.Recipient().String()

	data, err := json.Marshal(keyJson)
	if err != nil {
		return
	}
	err = os.WriteFile(keysFile, data, 0644)
	if err != nil {
		return
	}

	key.Recipient = identity.Recipient()

	for _, s := range keyJson.Identities {
		var id *age.X25519Identity
		id, err = age.ParseX25519Identity(s)
		if err != nil {
			return
		}
		key.Identities = append(key.Identities, id)
	}

	return
}
