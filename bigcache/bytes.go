// +build !appengine

package bigcache

import (
	"reflect"
	"unsafe"
)

func bytesToString(b []byte) string {
	// 这跟 string(b) 有啥区别？
	bytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	strHeader := reflect.StringHeader{Data: bytesHeader.Data, Len: bytesHeader.Len}
	return *(*string)(unsafe.Pointer(&strHeader))
}
