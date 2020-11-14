package bigcache

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/allegro/bigcache/v2/queue"
)

type onRemoveCallback func(wrappedEntry []byte, reason RemoveReason)

// Metadata contains information of a specific entry
// Metadata 包含 指定key的information
type Metadata struct {
	RequestCount uint32
}

type cacheShard struct {
	hashmap     map[uint64]uint32 //key的hash值和value的偏移的一个映射关系
	entries     queue.BytesQueue  // value都存在这
	lock        sync.RWMutex      //锁hashmap的
	entryBuffer []byte
	onRemove    onRemoveCallback

	isVerbose    bool
	statsEnabled bool
	logger       Logger
	clock        clock
	lifeWindow   uint64 //每个key的生存时间（就过期时间）

	hashmapStats map[uint64]uint32 //就记录了一下 hit 的次数，然后会在delete key的时候删除掉（记录了当前所有key的hit次数）
	stats        Stats
}

//从 shard 中get值，用key和hashedkey，拿到 entry ， Response， err。 Response会告知是否过期
func (s *cacheShard) getWithInfo(key string, hashedKey uint64) (entry []byte, resp Response, err error) {
	currentTime := uint64(s.clock.Epoch())            //这个就是当前时间
	s.lock.RLock()                                    //加读锁
	wrappedEntry, err := s.getWrappedEntry(hashedKey) //获取到从[]byte中存的entry
	if err != nil {
		s.lock.RUnlock()
		return nil, resp, err
	}
	if entryKey := readKeyFromEntry(wrappedEntry); key != entryKey { //从entry中读取 key，判断key是否一致。不一致就是发生了hash碰撞（这里涉及到一个如何解决hash碰撞的问题）
		s.lock.RUnlock()
		s.collision()    //记录一下发生了hash碰撞
		if s.isVerbose { //打日志
			s.logger.Printf("Collision detected. Both %q and %q have the same hash %x", key, entryKey, hashedKey)
		}
		return nil, resp, ErrEntryNotFound
	}

	entry = readEntry(wrappedEntry)                         //读出entry
	oldestTimeStamp := readTimestampFromEntry(wrappedEntry) //从entry中读出时间戳
	s.lock.RUnlock()
	s.hit(hashedKey) //缓存命中
	if currentTime-oldestTimeStamp >= s.lifeWindow {
		resp.EntryStatus = Expired //提示过期
	}
	return entry, resp, nil
}

//从 shard 中 get值，过期的也会返回。
func (s *cacheShard) get(key string, hashedKey uint64) ([]byte, error) {
	s.lock.RLock()
	wrappedEntry, err := s.getWrappedEntry(hashedKey)
	if err != nil {
		s.lock.RUnlock()
		return nil, err
	}
	if entryKey := readKeyFromEntry(wrappedEntry); key != entryKey {
		s.lock.RUnlock()
		s.collision()
		if s.isVerbose {
			s.logger.Printf("Collision detected. Both %q and %q have the same hash %x", key, entryKey, hashedKey)
		}
		return nil, ErrEntryNotFound
	}
	entry := readEntry(wrappedEntry)
	s.lock.RUnlock()
	s.hit(hashedKey)

	return entry, nil
}

//用hashedKey获取到存在[]byte数组中的entry
func (s *cacheShard) getWrappedEntry(hashedKey uint64) ([]byte, error) {
	itemIndex := s.hashmap[hashedKey]

	if itemIndex == 0 {
		s.miss()
		return nil, ErrEntryNotFound
	}

	//去byte数组中拿，itemIndex是开始索引。
	wrappedEntry, err := s.entries.Get(int(itemIndex))
	if err != nil {
		s.miss()
		return nil, err
	}

	return wrappedEntry, err
}

//用hashedKey获取到存在[]byte数组中的合法的entry，合法就是说key和取出的key是否一致。不一致就是冲突了
func (s *cacheShard) getValidWrapEntry(key string, hashedKey uint64) ([]byte, error) {
	wrappedEntry, err := s.getWrappedEntry(hashedKey)
	if err != nil {
		return nil, err
	}

	if !compareKeyFromEntry(wrappedEntry, key) {
		s.collision()
		if s.isVerbose {
			s.logger.Printf("Collision detected. Both %q and %q have the same hash %x", key, readKeyFromEntry(wrappedEntry), hashedKey)
		}

		return nil, ErrEntryNotFound
	}
	s.hitWithoutLock(hashedKey)

	return wrappedEntry, nil
}

//放进byte数组中。在set的时候，会检查entries中最老的entry是否已经过期，如果过期就删除最老的key
//如果没有空间存新的entry了，会一次次删除最老的entry，直到能够存的下
//这里要注意一点，如果存的entry过大，导致整个shard都存不下了，会直接导致shard被清空，shard被扩容到最大，然后才会报错整个shard都存不下（todo 是不是可以提前判断？）
func (s *cacheShard) set(key string, hashedKey uint64, entry []byte) error {
	currentTimestamp := uint64(s.clock.Epoch()) //当前时间

	s.lock.Lock()

	//如果原来已经存在该hashedKey，就取出原来的entry，然后将entry中存的key重置了（就是置成了空数组）
	if previousIndex := s.hashmap[hashedKey]; previousIndex != 0 {
		if previousEntry, err := s.entries.Get(int(previousIndex)); err == nil {
			resetKeyFromEntry(previousEntry) //这里就是标记一下，entries中该entry已经不可用了。todo ，那onEvict的时候是不是发现不可用可以删？现在只是通过过期时间删除
		}
	}

	//这个意思就是，在每次set操作的时候，都会从entries中拿出最老的，判断是否过期，如果过期就删除，不过期就啥都不做
	if oldestEntry, err := s.entries.Peek(); err == nil {
		s.onEvict(oldestEntry, currentTimestamp, s.removeOldestEntry)
	}

	//encode
	w := wrapEntry(currentTimestamp, hashedKey, key, entry, &s.entryBuffer)

	for {
		if index, err := s.entries.Push(w); err == nil {
			//push成功，记录一下，返回
			s.hashmap[hashedKey] = uint32(index)
			s.lock.Unlock()
			return nil
		}
		//因没有空间删除。也就是如果新加入的key没有了空间，会删除最老的entry，直到有空间存新的entry为止。
		if s.removeOldestEntry(NoSpace) != nil {
			s.lock.Unlock()
			return fmt.Errorf("entry is bigger than max shard size")
		}
	}
}

// set 不加锁。加的都是新的
func (s *cacheShard) addNewWithoutLock(key string, hashedKey uint64, entry []byte) error {
	currentTimestamp := uint64(s.clock.Epoch())

	if oldestEntry, err := s.entries.Peek(); err == nil {
		s.onEvict(oldestEntry, currentTimestamp, s.removeOldestEntry)
	}

	w := wrapEntry(currentTimestamp, hashedKey, key, entry, &s.entryBuffer)

	for {
		if index, err := s.entries.Push(w); err == nil {
			s.hashmap[hashedKey] = uint32(index)
			return nil
		}
		if s.removeOldestEntry(NoSpace) != nil {
			return fmt.Errorf("entry is bigger than max shard size")
		}
	}
}

// 直接给encode好的entry，然后不加锁set
func (s *cacheShard) setWrappedEntryWithoutLock(currentTimestamp uint64, w []byte, hashedKey uint64) error {
	if previousIndex := s.hashmap[hashedKey]; previousIndex != 0 {
		if previousEntry, err := s.entries.Get(int(previousIndex)); err == nil {
			resetKeyFromEntry(previousEntry)
		}
	}

	if oldestEntry, err := s.entries.Peek(); err == nil {
		s.onEvict(oldestEntry, currentTimestamp, s.removeOldestEntry)
	}

	for {
		if index, err := s.entries.Push(w); err == nil {
			s.hashmap[hashedKey] = uint32(index)
			return nil
		}
		if s.removeOldestEntry(NoSpace) != nil {
			return fmt.Errorf("entry is bigger than max shard size")
		}
	}
}

func (s *cacheShard) append(key string, hashedKey uint64, entry []byte) error {
	s.lock.Lock()
	wrappedEntry, err := s.getValidWrapEntry(key, hashedKey) //取出entry

	if err == ErrEntryNotFound {
		//如果本来就没有，就新增一个key。因为本身加锁了，所以这里调用的是不加锁的函数
		err = s.addNewWithoutLock(key, hashedKey, entry)
		s.lock.Unlock()
		return err
	}
	if err != nil {
		s.lock.Unlock()
		return err
	}

	currentTimestamp := uint64(s.clock.Epoch())

	//todo 这里没看懂，干嘛要这样？
	w := appendToWrappedEntry(currentTimestamp, wrappedEntry, entry, &s.entryBuffer)

	//将已经warp的存起来
	err = s.setWrappedEntryWithoutLock(currentTimestamp, w, hashedKey)
	s.lock.Unlock()

	return err
}

//会先检查是否有，没有直接返回，有才会删除
func (s *cacheShard) del(hashedKey uint64) error {
	// Optimistic pre-check using only readlock
	//仅使用readlock进行乐观的预检查
	s.lock.RLock()
	{
		itemIndex := s.hashmap[hashedKey]
		// hashmap中本来就没有该key
		if itemIndex == 0 {
			s.lock.RUnlock()
			s.delmiss()
			return ErrEntryNotFound
		}
		//hashmap中有，但是entries中没有
		if err := s.entries.CheckGet(int(itemIndex)); err != nil {
			s.lock.RUnlock()
			s.delmiss()
			return err
		}
	}
	s.lock.RUnlock()

	s.lock.Lock()
	{
		// After obtaining the writelock, we need to read the same again,
		// since the data delivered earlier may be stale now
		//获得写锁后，我们需要再次读取该写锁，因为之前传递的数据现在可能已过时
		itemIndex := s.hashmap[hashedKey]

		if itemIndex == 0 {
			s.lock.Unlock()
			s.delmiss()
			return ErrEntryNotFound
		}

		wrappedEntry, err := s.entries.Get(int(itemIndex))
		if err != nil {
			s.lock.Unlock()
			s.delmiss()
			return err
		}

		delete(s.hashmap, hashedKey)
		s.onRemove(wrappedEntry, Deleted)
		if s.statsEnabled {
			delete(s.hashmapStats, hashedKey)
		}
		resetKeyFromEntry(wrappedEntry)
	}
	s.lock.Unlock()

	s.delhit()
	return nil
}

//删除key
func (s *cacheShard) onEvict(oldestEntry []byte, currentTimestamp uint64, evict func(reason RemoveReason) error) bool {
	oldestTimestamp := readTimestampFromEntry(oldestEntry)
	if currentTimestamp-oldestTimestamp > s.lifeWindow {
		evict(Expired)
		return true
	}
	return false
}

//删除所有过期的key
func (s *cacheShard) cleanUp(currentTimestamp uint64) {
	s.lock.Lock()
	for {
		if oldestEntry, err := s.entries.Peek(); err != nil {
			break
		} else if evicted := s.onEvict(oldestEntry, currentTimestamp, s.removeOldestEntry); !evicted {
			break
		}
	}
	s.lock.Unlock()
}

// 就是把从entries中拿出来的byte数组拷贝了一份出来，然后返回
func (s *cacheShard) getEntry(hashedKey uint64) ([]byte, error) {
	s.lock.RLock()

	entry, err := s.getWrappedEntry(hashedKey)
	// copy entry
	newEntry := make([]byte, len(entry))
	copy(newEntry, entry)

	s.lock.RUnlock()

	return newEntry, err
}

//将hashed key拷贝出去，返回所有的hashed key，以及个数
func (s *cacheShard) copyHashedKeys() (keys []uint64, next int) {
	s.lock.RLock()
	keys = make([]uint64, len(s.hashmap))

	for key := range s.hashmap {
		keys[next] = key
		next++
	}

	s.lock.RUnlock()
	return keys, next
}

//删除最老的entry
func (s *cacheShard) removeOldestEntry(reason RemoveReason) error {
	oldest, err := s.entries.Pop()
	if err == nil {
		hash := readHashFromEntry(oldest) //set的时候发生碰撞后reset key是在这用到的。
		if hash == 0 {
			// entry has been explicitly deleted with resetKeyFromEntry, ignore
			// 条目已使用resetKeyFromEntry明确删除，请忽略
			return nil
		}
		delete(s.hashmap, hash)
		s.onRemove(oldest, reason)
		if s.statsEnabled {
			delete(s.hashmapStats, hash)
		}
		return nil
	}
	return err
}

//重置
func (s *cacheShard) reset(config Config) {
	s.lock.Lock()
	s.hashmap = make(map[uint64]uint32, config.initialShardSize())
	s.entryBuffer = make([]byte, config.MaxEntrySize+headersSizeInBytes)
	s.entries.Reset()
	s.lock.Unlock()
}

//返回当前存的key的个数
func (s *cacheShard) len() int {
	s.lock.RLock()
	res := len(s.hashmap)
	s.lock.RUnlock()
	return res
}

//返回当前容量
func (s *cacheShard) capacity() int {
	s.lock.RLock()
	res := s.entries.Capacity()
	s.lock.RUnlock()
	return res
}

func (s *cacheShard) getStats() Stats {
	var stats = Stats{
		Hits:       atomic.LoadInt64(&s.stats.Hits),
		Misses:     atomic.LoadInt64(&s.stats.Misses),
		DelHits:    atomic.LoadInt64(&s.stats.DelHits),
		DelMisses:  atomic.LoadInt64(&s.stats.DelMisses),
		Collisions: atomic.LoadInt64(&s.stats.Collisions),
	}
	return stats
}

func (s *cacheShard) getKeyMetadataWithLock(key uint64) Metadata {
	s.lock.RLock()
	c := s.hashmapStats[key]
	s.lock.RUnlock()
	return Metadata{
		RequestCount: c,
	}
}

func (s *cacheShard) getKeyMetadata(key uint64) Metadata {
	return Metadata{
		RequestCount: s.hashmapStats[key],
	}
}

func (s *cacheShard) hit(key uint64) {
	atomic.AddInt64(&s.stats.Hits, 1)
	if s.statsEnabled {
		s.lock.Lock()
		s.hashmapStats[key]++
		s.lock.Unlock()
	}
}

func (s *cacheShard) hitWithoutLock(key uint64) {
	atomic.AddInt64(&s.stats.Hits, 1)
	if s.statsEnabled {
		s.hashmapStats[key]++
	}
}

func (s *cacheShard) miss() {
	atomic.AddInt64(&s.stats.Misses, 1)
}

func (s *cacheShard) delhit() {
	atomic.AddInt64(&s.stats.DelHits, 1)
}

func (s *cacheShard) delmiss() {
	atomic.AddInt64(&s.stats.DelMisses, 1)
}

func (s *cacheShard) collision() {
	atomic.AddInt64(&s.stats.Collisions, 1)
}

//在初始化bigCache的时候会调用。这里的参数callback是个函数变量。在bigcache中实现了3个该函数可以选择性传。
func initNewShard(config Config, callback onRemoveCallback, clock clock) *cacheShard {
	bytesQueueInitialCapacity := config.initialShardSize() * config.MaxEntrySize //单个shard的最大entry数+最大entry size
	maximumShardSizeInBytes := config.maximumShardSizeInBytes()
	if maximumShardSizeInBytes > 0 && bytesQueueInitialCapacity > maximumShardSizeInBytes {
		bytesQueueInitialCapacity = maximumShardSizeInBytes //以maximumShardSizeInBytes为主
	}
	return &cacheShard{
		hashmap:      make(map[uint64]uint32, config.initialShardSize()), //单个shard的最大entry数个大小
		hashmapStats: make(map[uint64]uint32, config.initialShardSize()),
		//entries 是一个可扩展的byte队列，初始 bytesQueueInitialCapacity， 最大 maximumShardSizeInBytes
		entries:     *queue.NewBytesQueue(bytesQueueInitialCapacity, maximumShardSizeInBytes, config.Verbose),
		entryBuffer: make([]byte, config.MaxEntrySize+headersSizeInBytes), //这个buffer可以存一个entry，应该是用来避免重复开辟内存的，类似缓存池
		onRemove:    callback,                                             //删除entry后的回调

		isVerbose:    config.Verbose,
		logger:       newLogger(config.Logger),
		clock:        clock,
		lifeWindow:   uint64(config.LifeWindow.Seconds()),
		statsEnabled: config.StatsEnabled,
	}
}
