//go:build goexperiment.arenas

package jmt

import (
	"reflect"
	"unsafe"
)

var _ [NodeSize - int(unsafe.Sizeof(Node{}))]byte
var _ [int(unsafe.Sizeof(Node{})) - NodeSize]byte

func hasForbiddenPointerKinds(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface, reflect.Func, reflect.Chan, reflect.String:
		return true
	case reflect.Array:
		return hasForbiddenPointerKinds(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if hasForbiddenPointerKinds(t.Field(i).Type) {
				return true
			}
		}
	}
	return false
}
