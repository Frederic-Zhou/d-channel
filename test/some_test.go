package test

import (
	"fmt"
	"testing"
)

func TestMap(t *testing.T) {

	m := map[string]string{"a": "b"}

	_, ok := m["a"]
	fmt.Println(ok)

	_, ok = m["b"]
	fmt.Println(ok)
}
