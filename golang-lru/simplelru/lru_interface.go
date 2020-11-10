// Package simplelru provides simple LRU implementation based on build-in container/list.
package simplelru

// LRUCache is the interface for simple LRU cache.
// simple LRU cache 的接口
type LRUCache interface {
	// Adds a value to the cache, returns true if an eviction occurred and
	// updates the "recently used"-ness of the key.
	//向缓存中添加一个值，如果发生逐出，则返回true，并更新密钥的“最近使用”状态。
	Add(key, value interface{}) bool

	// Returns key's value from the cache and
	// updates the "recently used"-ness of the key. #value, isFound
	// 返回key的value，并更新lru
	Get(key interface{}) (value interface{}, ok bool)

	// Checks if a key exists in cache without updating the recent-ness.
	// 检查key是否存在，不更新lru
	Contains(key interface{}) (ok bool)

	// Returns key's value without updating the "recently used"-ness of the key.
	// 返回key的value，不跟新lru
	Peek(key interface{}) (value interface{}, ok bool)

	// Removes a key from the cache.
	// 删除key
	Remove(key interface{}) bool

	// Removes the oldest entry from cache.
	// 删除最旧的cache
	RemoveOldest() (interface{}, interface{}, bool)

	// Returns the oldest entry from the cache. #key, value, isFound
	// 返回最旧的key value
	GetOldest() (interface{}, interface{}, bool)

	// Returns a slice of the keys in the cache, from oldest to newest.
	// 返回cache中所有的key，从老到新
	Keys() []interface{}

	// Returns the number of items in the cache.
	Len() int

	// Clears all cache entries.
	// 清空cache
	Purge()

	// Resizes cache, returning number evicted
	// 调整缓存大小，返回逐出的数字
	Resize(int) int
}
