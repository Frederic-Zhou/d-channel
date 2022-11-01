package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"filippo.io/age"
	"filippo.io/age/armor"
)

const keysFile = "./secretkeys.json"

var secretKeys *SecretKeys

func GetSecretKey(password string) (*SecretKeys, error) {

	//从文件中读取key
	//如果文件不存在，创建
	//如果存在，读取到keyJson，然后通过age.Parse方法，将文本的key转换为对象返回
	keysfile, err := os.ReadFile(keysFile)
	keyJson := keyJson{}
	if os.IsNotExist(err) { //如果不存在错误，创建新key，并写入到keyJson
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			return nil, fmt.Errorf("failed to spawn age indetity: %s", err.Error())
		}

		keyJson.Identities = []string{identity.String()}
		keyJson.Recipient = identity.Recipient().String()

		data, err := json.Marshal(keyJson)
		if err != nil {
			return nil, fmt.Errorf("failed to json format keys %s", err.Error())
		}

		scryptRecipient, err := age.NewScryptRecipient(password)
		if err != nil {
			return nil, fmt.Errorf("failed to NewScryptRecipient %s", err.Error())
		}

		kfbuf := bytes.NewBuffer(data)
		out := bytes.NewBuffer([]byte{})
		err = Encrypt([]age.Recipient{scryptRecipient}, kfbuf, out)
		if err != nil {
			return nil, fmt.Errorf("failed to password encrypt %s", err.Error())
		}

		err = os.WriteFile(keysFile, out.Bytes(), 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to write key store %s", err.Error())
		}

	} else if err != nil { //如果是非不存在的其他错误
		return nil, fmt.Errorf("failed to read %s", err.Error())
	} else { //存在，并且没有错误，用文件转换为keyJson
		scryptIdentity, err := age.NewScryptIdentity(password)
		if err != nil {
			return nil, fmt.Errorf("failed to NewScryptIdentity %s", err.Error())
		}
		kfbuf := bytes.NewBuffer(keysfile)
		out := bytes.NewBuffer([]byte{})
		err = Decrypt([]age.Identity{scryptIdentity}, kfbuf, out)
		if err != nil {
			return nil, fmt.Errorf("failed to password decrypt %s", err.Error())
		}

		err = json.Unmarshal(out.Bytes(), &keyJson)
		if err != nil {
			return nil, fmt.Errorf("failed to Unmarshal %s", err.Error())
		}
	}

	secretKeys = &SecretKeys{}
	secretKeys.Recipient, err = age.ParseX25519Recipient(keyJson.Recipient)
	if err != nil {
		return nil, fmt.Errorf("failed to read key Recipient %s", err.Error())
	}

	for _, s := range keyJson.Identities {
		id, err := age.ParseX25519Identity(s)
		if err != nil {
			return nil, fmt.Errorf("failed to read identities %s", err.Error())
		}
		secretKeys.Identities = append(secretKeys.Identities, id)
	}

	return secretKeys, nil

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

// NewSecretKey 会保留原来的私钥，新增一对公私钥
func NewSecretKey(passwordtoEncrypt string) (*SecretKeys, error) {

	//生成新的
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("failed to spawn age indetity: %s", err.Error())
	}

	secretKeys.Recipient = identity.Recipient()
	secretKeys.Identities = append(secretKeys.Identities, identity)

	keyjson := keyJson{}
	keyjson.Recipient = secretKeys.Recipient.(*age.X25519Recipient).String()
	for _, s := range secretKeys.Identities {
		keyjson.Identities = append(keyjson.Identities, s.(*age.X25519Identity).String())
	}

	data, err := json.Marshal(keyjson)
	if err != nil {
		return nil, err
	}

	scryptRecipient, err := age.NewScryptRecipient(passwordtoEncrypt)
	if err != nil {
		return nil, fmt.Errorf("failed to NewScryptRecipient %s", err.Error())
	}

	kfbuf := bytes.NewBuffer(data)
	out := bytes.NewBuffer([]byte{})
	err = Encrypt([]age.Recipient{scryptRecipient}, kfbuf, out)
	if err != nil {
		return nil, fmt.Errorf("failed to password encrypt %s", err.Error())
	}

	err = os.WriteFile(keysFile, data, 0644)
	if err != nil {
		return nil, err
	}

	return secretKeys, err
}
