package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"filippo.io/age"
	"filippo.io/age/armor"
)

func GetAge() (keys []Key) {

	//创建目录，如果不存在
	keystore, err := createKeystore()
	if err != nil {
		panic(fmt.Errorf("failed to create key store"))
	}

	//查询目录中所有key文件
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

		fbytes := bytes.Split(keyfile, []byte("\n"))
		if len(fbytes) != 2 {
			return nil
		}

		re, err := age.ParseX25519Recipient(string(fbytes[0]))
		if err != nil {
			return nil
		}
		id, err := age.ParseX25519Identity(string(fbytes[1]))
		if err != nil {
			return nil
		}

		keys = append(keys, Key{Name: filepath.Base(path), Recipient: re, Identity: id})

		return nil
	})

	//如果一个都没有，创建一个
	if len(keys) == 0 {
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			panic(fmt.Errorf("failed to spawn age indetity: %s", err))
		}

		key := Key{
			Name:      "self",
			Identity:  identity,
			Recipient: identity.Recipient(),
		}

		err = os.WriteFile(filepath.Join(keystore, key.Name),
			bytes.Join([][]byte{
				[]byte(identity.Recipient().String()),
				[]byte(identity.String()),
			}, []byte("\n")), 0644)
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

type Key struct {
	Name      string
	Identity  *age.X25519Identity
	Recipient *age.X25519Recipient
}

func Encrypt(recipients []age.Recipient, in io.Reader, out io.Writer, withArmor bool) error {
	if withArmor {
		a := armor.NewWriter(out)
		defer func() {
			if err := a.Close(); err != nil {
				return
			}
		}()
		out = a
	}
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
	if intro, _ := rr.Peek(len(crlfMangledIntro)); string(intro) == crlfMangledIntro ||
		string(intro) == utf16MangledIntro {
		return fmt.Errorf("%s%s%s",
			"invalid header intro",
			"it looks like this file was corrupted by PowerShell redirection",
			"consider using -o or -a to encrypt files in PowerShell")
	}

	if start, _ := rr.Peek(len(armor.Header)); string(start) == armor.Header {
		in = armor.NewReader(rr)
	} else {
		in = rr
	}

	r, err := age.Decrypt(in, identities...)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	return nil
}

const crlfMangledIntro = "age-encryption.org/v1" + "\r"
const utf16MangledIntro = "\xff\xfe" + "a\x00g\x00e\x00-\x00e\x00n\x00c\x00r\x00y\x00p\x00"
