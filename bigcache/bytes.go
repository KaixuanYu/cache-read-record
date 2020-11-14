// +build !appengine

package bigcache

import (
	"reflect"
	"unsafe"
)

func bytesToString(b []byte) string {
	// 这跟 string(b) 有啥区别？只是指针的转换，没有内存的拷贝。如果直接string(b)会重新开辟内存。然后重新拷贝过去。
	bytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	strHeader := reflect.StringHeader{Data: bytesHeader.Data, Len: bytesHeader.Len}
	return *(*string)(unsafe.Pointer(&strHeader))
}
