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

	keysfile, err := os.ReadFile(keysFile)
	keyMap := keyMap{}
	if os.IsNotExist(err) {
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			panic(fmt.Errorf("failed to spawn age indetity: %s", err))
		}

		keyMap.Identities = []string{identity.String()}
		keyMap.Recipient = identity.Recipient().String()

		data, err := json.Marshal(keyMap)
		if err != nil {
			panic(fmt.Errorf("failed to json format keys"))
		}
		err = os.WriteFile(keysFile, data, 0644)
		if err != nil {
			panic(fmt.Errorf("failed to write key store"))
		}

		return
	}

	json.Unmarshal(keysfile, &keyMap)

	key.Recipient, err = age.ParseX25519Recipient(keyMap.Recipient)
	if err != nil {
		panic(fmt.Errorf("failed to read key Recipient %s", err.Error()))
	}

	for _, s := range keyMap.Identities {
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
type keyMap struct {
	Identities []string `json:"identities"`
	Recipient  string   `json:"recipient"`
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
