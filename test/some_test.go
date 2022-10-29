package test

import (
	"fmt"
	"reflect"
	"testing"
)

func TestStructType(t *testing.T) {

	m := map[string]interface{}{"a": "b", "v": 1}

	type1 := reflect.TypeOf(m)

	str := []float64{13.21}
	fmt.Printf("%s\n%v\n%+v\n%#v\n%T\n", type1.String(), m, m, m, m)
	fmt.Printf("%v\n%+v\n%#v\n%T\n", str, str, str, str)

}
