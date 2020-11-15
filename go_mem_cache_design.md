# go语言如何设计内存存储

[toc]

阅读完一些go实现的内存缓存包之后，来聊一聊该如何用go设计内存存储系统。

我们从简单到复杂逐步来：

## go-cache 最简单的内存缓存实现

要用 go 来实现一个 `key value` 对的存储系统，能直接想到用 `map` 来存。

`key` 是一个字符串， `value` 可能使任意结构体。
我们用 `entry` 代表一个 `key value`对（实际 `go-cache` 用的 `item` 来代表），就可以定义成如下结构：
```
entries map[string]interface{}
```

因为 `go` 语言中 `map` 是并发不安全的，所以我们需要给 `map` 加个读写锁。结构就变成了这样：
```
type cache struct {
	mu sync.RWMutex
	entries map[string]interface{}
}
```

这时候，并发的增删改查就可以实现了。接下来我不想每次都手动删除`entry`,我想给每个 `entry` 一个过期时间，让他们过期自动删除，那么结构就变成这样：
```
type cache struct {
	mu sync.RWMutex
	entries map[string]Entry
}
type Entry struct {
    Value      interface{}
    Expiration int64
}
```
我给每一个`entry`都增加了一个过期时间的属性。
只要在创建`cache`结构其的时候，开个协程跑个定时任务，用来遍历 `map` ，删除过期的`entry`就可以了。

到这里 `go-cache` 包的基本功能都有了。然后 `go-cache` 还额外做了如下内容：

- 给 `cache` 增加了个额外的 `defaultExpiration time.Duration ` 属性，可以不用创建一个 `entry` 都传过期时间的参数。
- 增加了 `onEvicted func(string, interface{})` 属性，用户可以自己设置该属性，用来给当 `entry` 删除的时候调用，来做某些操作。
- 自动删除停止协程定时任务，不需要在创建之后马上`defer cache.Stop` ，如何实现的具体如下：

`go-cache` 在 `cache` 结构之后又包了一层，变成这样：
```
tyep Cache struct {
    *cache
}
```
其目的在以下代码中：
```
func newCacheWithJanitor(de time.Duration, ci time.Duration, m map[string]Item) *Cache {
	c := newCache(de, m)
    
    C := &Cache{c}
	if ci > 0 {
		runJanitor(c, ci)
		runtime.SetFinalizer(C, stopJanitor) //此函数的意义：当GC准备释放C时，会调用 stopJanitor 方法。
	}
	return C
}
```
其意义就是，对外部给的都是 `Cache` ，当 `Cache` 不再被应用的时候，GC会删除它，然后顺便会执行 `stopJanitor` 函数，
用来停止协程定时任务。