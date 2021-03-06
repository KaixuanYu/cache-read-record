package bigcache

import (
	"fmt"
	"time"
)

const (
	minimumEntriesInShard = 10 // Minimum number of entries in single shard 在单个 shard 中entries最小的数量
)

// BigCache is fast, concurrent, evicting cache created to keep big number of entries without impact on performance.
// It keeps entries on heap but omits GC for them. To achieve that, operations take place on byte arrays,
// therefore entries (de)serialization in front of the cache will be needed in most use cases.
type BigCache struct {
	shards       []*cacheShard
	lifeWindow   uint64
	clock        clock
	hash         Hasher
	config       Config
	shardMask    uint64
	maxShardSize uint32
	close        chan struct{}
}

// Response will contain metadata about the entry for which GetWithInfo(key) was called
// 响应将包含有关调用了GetWithInfo（key）的条目的元数据
type Response struct {
	EntryStatus RemoveReason
}

// RemoveReason is a value used to signal to the user why a particular key was removed in the OnRemove callback.
// 用来标识为啥指定的key在OnRemove回调中删除
type RemoveReason uint32

const (
	// Expired means the key is past its LifeWindow.
	// 过期
	Expired = RemoveReason(1)
	// NoSpace means the key is the oldest and the cache size was at its maximum when Set was called, or the
	// entry exceeded the maximum shard size.
	// 空间不足，意味着key是最老的那个，并且cache size 已经达到我们设置的上限了，或者条目超出了shard的最大上限
	NoSpace = RemoveReason(2)
	// Deleted means Delete was called and this key was removed as a result.
	// Deleted 意味着 Delete 被调用
	Deleted = RemoveReason(3)
)

// NewBigCache initialize new instance of BigCache
// NewBigCache 初始化新的 bigCache 实例
func NewBigCache(config Config) (*BigCache, error) {
	return newBigCache(config, &systemClock{})
}

func newBigCache(config Config, clock clock) (*BigCache, error) {

	if !isPowerOfTwo(config.Shards) { //检查是否是2次幂
		return nil, fmt.Errorf("Shards number must be power of two")
	}

	if config.Hasher == nil {
		config.Hasher = newDefaultHasher()
	}

	cache := &BigCache{
		shards:       make([]*cacheShard, config.Shards),
		lifeWindow:   uint64(config.LifeWindow.Seconds()),
		clock:        clock,
		hash:         config.Hasher,
		config:       config,
		shardMask:    uint64(config.Shards - 1),
		maxShardSize: uint32(config.maximumShardSizeInBytes()),
		close:        make(chan struct{}),
	}

	var onRemove func(wrappedEntry []byte, reason RemoveReason)
	if config.OnRemoveWithMetadata != nil {
		onRemove = cache.providedOnRemoveWithMetadata
	} else if config.OnRemove != nil {
		onRemove = cache.providedOnRemove
	} else if config.OnRemoveWithReason != nil {
		onRemove = cache.providedOnRemoveWithReason
	} else {
		onRemove = cache.notProvidedOnRemove
	}

	//初始化各个shards
	for i := 0; i < config.Shards; i++ {
		cache.shards[i] = initNewShard(config, onRemove, clock)
	}

	//定时cleanUp
	if config.CleanWindow > 0 {
		go func() {
			ticker := time.NewTicker(config.CleanWindow)
			defer ticker.Stop()
			for {
				select {
				case t := <-ticker.C:
					cache.cleanUp(uint64(t.Unix()))
				case <-cache.close:
					return
				}
			}
		}()
	}

	return cache, nil
}

// Close is used to signal a shutdown of the cache when you are done with it.
// This allows the cleaning goroutines to exit and ensures references are not
// kept to the cache preventing GC of the entire cache.
// Close 用于在完成缓存后发出关闭信号。
// 这允许清理goroutines退出，并确保不会因保留对缓存的引用而阻止整个缓存的GC
func (c *BigCache) Close() error {
	close(c.close)
	return nil
}

// Get reads entry for the key.
// It returns an ErrEntryNotFound when
// no entry exists for the given key.
// Get函数读取key为key的entry。如果不存在就返回 ErrEntryNotFound
func (c *BigCache) Get(key string) ([]byte, error) {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.get(key, hashedKey)
}

// GetWithInfo reads entry for the key with Response info.
// It returns an ErrEntryNotFound when
// no entry exists for the given key.
// Response info 里面只有删除的类型，这是会返回软删除的key吗？是的，getWithInfo会返回已经过期但是未删除的key，并且Response会标记已经删除
func (c *BigCache) GetWithInfo(key string) ([]byte, Response, error) {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.getWithInfo(key, hashedKey)
}

// Set saves entry under the key
// Set 储存一个entry
func (c *BigCache) Set(key string, entry []byte) error {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.set(key, hashedKey, entry)
}

// Append appends entry under the key if key exists, otherwise
// it will set the key (same behaviour as Set()). With Append() you can
// concatenate multiple entries under the same key in an lock optimized way.
// 如果 key 存在，Append 会在指定key下追加一个entry，否则就set一个key（就成了Set()）。
// 使用Append（）可以以锁优化的方式连接同一个键下的多个条目。
func (c *BigCache) Append(key string, entry []byte) error {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.append(key, hashedKey, entry)
}

// Delete removes the key
func (c *BigCache) Delete(key string) error {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.del(hashedKey)
}

// Reset empties all cache shards
// Reset 清空所有的 缓存分片
func (c *BigCache) Reset() error {
	for _, shard := range c.shards {
		shard.reset(c.config)
	}
	return nil
}

// Len computes number of entries in cache
// Len 返回所有分片中的entries的总和
func (c *BigCache) Len() int {
	var len int
	for _, shard := range c.shards {
		len += shard.len()
	}
	return len
}

// Capacity returns amount of bytes store in the cache.
// Capacity 返回缓存中存储的字节数。
func (c *BigCache) Capacity() int {
	var len int
	for _, shard := range c.shards {
		len += shard.capacity()
	}
	return len
}

// Stats returns cache's statistics 返回cache统计信息
func (c *BigCache) Stats() Stats {
	var s Stats
	for _, shard := range c.shards {
		tmp := shard.getStats()
		s.Hits += tmp.Hits
		s.Misses += tmp.Misses
		s.DelHits += tmp.DelHits
		s.DelMisses += tmp.DelMisses
		s.Collisions += tmp.Collisions
	}
	return s
}

// KeyMetadata returns number of times a cached resource was requested.
//KeyMetadata返回请求缓存资源的次数。
func (c *BigCache) KeyMetadata(key string) Metadata {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.getKeyMetadataWithLock(hashedKey)
}

// Iterator returns iterator function to iterate over EntryInfo's from whole cache.
//迭代器返回迭代器函数以迭代整个缓存中的EntryInfo。
func (c *BigCache) Iterator() *EntryInfoIterator {
	return newIterator(c)
}

func (c *BigCache) onEvict(oldestEntry []byte, currentTimestamp uint64, evict func(reason RemoveReason) error) bool {
	oldestTimestamp := readTimestampFromEntry(oldestEntry)
	if currentTimestamp-oldestTimestamp > c.lifeWindow {
		evict(Expired)
		return true
	}
	return false
}

//cleanUp清除所有的过期key value
func (c *BigCache) cleanUp(currentTimestamp uint64) {
	for _, shard := range c.shards {
		shard.cleanUp(currentTimestamp)
	}
}

func (c *BigCache) getShard(hashedKey uint64) (shard *cacheShard) {
	return c.shards[hashedKey&c.shardMask]
}

func (c *BigCache) providedOnRemove(wrappedEntry []byte, reason RemoveReason) {
	c.config.OnRemove(readKeyFromEntry(wrappedEntry), readEntry(wrappedEntry))
}

func (c *BigCache) providedOnRemoveWithReason(wrappedEntry []byte, reason RemoveReason) {
	if c.config.onRemoveFilter == 0 || (1<<uint(reason))&c.config.onRemoveFilter > 0 {
		c.config.OnRemoveWithReason(readKeyFromEntry(wrappedEntry), readEntry(wrappedEntry), reason)
	}
}

func (c *BigCache) notProvidedOnRemove(wrappedEntry []byte, reason RemoveReason) {
}

func (c *BigCache) providedOnRemoveWithMetadata(wrappedEntry []byte, reason RemoveReason) {
	hashedKey := c.hash.Sum64(readKeyFromEntry(wrappedEntry))
	shard := c.getShard(hashedKey)
	c.config.OnRemoveWithMetadata(readKeyFromEntry(wrappedEntry), readEntry(wrappedEntry), shard.getKeyMetadata(hashedKey))
}
