package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestJson(t *testing.T) {

	data := map[string][]byte{
		"hello": []byte("world"),
	}

	body, err := json.Marshal(data)

	fmt.Println(string(body))
	fmt.Println(err)
}
