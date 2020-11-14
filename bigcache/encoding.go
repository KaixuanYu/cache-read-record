package bigcache

import (
	"encoding/binary"
)

const (
	timestampSizeInBytes = 8                                                       // Number of bytes used for timestamp 时间戳的字节数(int64 8字节)
	hashSizeInBytes      = 8                                                       // Number of bytes used for hash hash值的字节数(int64 8字节)
	keySizeInBytes       = 2                                                       // Number of bytes used for size of entry key
	headersSizeInBytes   = timestampSizeInBytes + hashSizeInBytes + keySizeInBytes // Number of bytes used for all headers entry头的总字节数
)

//封装entry，就是encode呗。 时间戳(8字节)+hash key(8字节)+key 长度(2字节) + key + value
func wrapEntry(timestamp uint64, hash uint64, key string, entry []byte, buffer *[]byte) []byte {
	keyLength := len(key)
	blobLength := len(entry) + headersSizeInBytes + keyLength

	if blobLength > len(*buffer) {
		*buffer = make([]byte, blobLength)
	}
	blob := *buffer

	binary.LittleEndian.PutUint64(blob, timestamp)                                                //前64位（8字节）是个时间戳
	binary.LittleEndian.PutUint64(blob[timestampSizeInBytes:], hash)                              // 接着64位（8字节）是key的hash值
	binary.LittleEndian.PutUint16(blob[timestampSizeInBytes+hashSizeInBytes:], uint16(keyLength)) //接着16位（2字节）是key的长度
	copy(blob[headersSizeInBytes:], key)                                                          // 放入key
	copy(blob[headersSizeInBytes+keyLength:], entry)                                              //放入value

	return blob[:blobLength]
}

// 在 wrappedEntry 上 追加 entry。1. 更新时间戳（前8字节），将原entry放到时间戳之后，然后在接上新的entry
func appendToWrappedEntry(timestamp uint64, wrappedEntry []byte, entry []byte, buffer *[]byte) []byte {
	blobLength := len(wrappedEntry) + len(entry)
	if blobLength > len(*buffer) {
		*buffer = make([]byte, blobLength)
	}

	blob := *buffer

	binary.LittleEndian.PutUint64(blob, timestamp)
	copy(blob[timestampSizeInBytes:], wrappedEntry[timestampSizeInBytes:])
	copy(blob[len(wrappedEntry):], entry)

	return blob[:blobLength]
}

//解析entry，就是decode呗。只取value
func readEntry(data []byte) []byte {
	//取key的长度
	length := binary.LittleEndian.Uint16(data[timestampSizeInBytes+hashSizeInBytes:])

	// copy on read 不取前面的时间戳+hash+key len+key，只要最后的value
	dst := make([]byte, len(data)-int(headersSizeInBytes+length))
	copy(dst, data[headersSizeInBytes+length:])

	return dst
}

//只取出entry的时间戳
func readTimestampFromEntry(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data)
}

//只取key
func readKeyFromEntry(data []byte) string {
	length := binary.LittleEndian.Uint16(data[timestampSizeInBytes+hashSizeInBytes:])

	// copy on read
	dst := make([]byte, length)
	copy(dst, data[headersSizeInBytes:headersSizeInBytes+length])

	return bytesToString(dst)
}

//判断data中的key是否等于 参数的key
func compareKeyFromEntry(data []byte, key string) bool {
	length := binary.LittleEndian.Uint16(data[timestampSizeInBytes+hashSizeInBytes:])

	return bytesToString(data[headersSizeInBytes:headersSizeInBytes+length]) == key
}

//从entry中读hash
func readHashFromEntry(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data[timestampSizeInBytes:])
}

//将hash key 的那64个字节清空了
func resetKeyFromEntry(data []byte) {
	binary.LittleEndian.PutUint64(data[timestampSizeInBytes:], 0)
}
