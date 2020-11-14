package bigcache

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 将MB转换成字节
func convertMBToBytes(value int) int {
	return value * 1024 * 1024
}

//判断是否是2的n次方
func isPowerOfTwo(number int) bool {
	return (number & (number - 1)) == 0
}
