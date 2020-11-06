# go-cache

## 简介
1. 支持并发。就是个加了读写锁的map。
2. 支持key过期。map的value中存了过期时间，启动个定时器遍历map过期删除（遍历的时候加了锁，key太多有性能问题）
3. 支持key过期事件。key过期会调用一个onEvicted函数，onEvicted是业务方自定义的。
4. 支持持久化。可以通过 Items()函数取出所有的key：value，然后自己做持久化存储。
5. 支持初始化加载指定的 key：value 的map

### 亮点
- 这里就是 在 cache 上包了一层 Cache，因为cache被runJanitor的goroutine引用，gc会一直忽略对它的回收。
- 但是包了一层Cache，当外部的使用方不再用Cache的时候，因为没有额外的引用，gc会回收Cache。
- SetFinalizer会在回收Cache的时候调用stopJanitor，停掉janitor goroutine，从而cache失去引用，也会被gc
```
type Cache struct {
	*cache
}
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

## 原readme阅读

go-cache is an in-memory key:value store/cache similar to memcached that is
suitable for applications running on a single machine. Its major advantage is
that, being essentially a thread-safe `map[string]interface{}` with expiration
times, it doesn't need to serialize or transmit its contents over the network.

go-cache 是一个基于内存的key：value储存，它类似于memcached，适用于在一台计算机上运行。它的主要优点是：
它本质上是具有到期时间的线程安全的 `map[string]interface` ，因此不需要序列化或者在网络上传输内容。

Any object can be stored, for a given duration or forever, and the cache can be
safely used by multiple goroutines.

可以储存任何对象（在给定的持续时间内或者永久储存），并且 缓存可以被多个 goroutines 安全的使用

Although go-cache isn't meant to be used as a persistent datastore, the entire
cache can be saved to and loaded from a file (using `c.Items()` to retrieve the
items map to serialize, and `NewFrom()` to create a cache from a deserialized
one) to recover from downtime quickly. (See the docs for `NewFrom()` for caveats.)

尽管不打算将go-cache用作持久数据储存数据库，但是可以将整个缓存保存到文件中，也可以从文件中加载出来。（使用 `c.Items` 来检索
items map 去 序列化，然后 `NewFrom()` 来创建一个cache从解序列化的那个。）

### Installation 安装

`go get github.com/patrickmn/go-cache`

### Usage 使用

```
import (
	"fmt"
	"github.com/patrickmn/go-cache"
	"time"
)

func main() {
	// Create a cache with a default expiration time of 5 minutes, and which
	// purges expired items every 10 minutes
	// 创建一个默认过期时间为5分钟的缓存，该缓存每10分钟清除一次过期的item
	c := cache.New(5*time.Minute, 10*time.Minute)

	// Set the value of the key "foo" to "bar", with the default expiration time
	c.Set("foo", "bar", cache.DefaultExpiration)

	// Set the value of the key "baz" to 42, with no expiration time
	// (the item won't be removed until it is re-set, or removed using
	// c.Delete("baz")
	c.Set("baz", 42, cache.NoExpiration)

	// Get the string associated with the key "foo" from the cache
	foo, found := c.Get("foo")
	if found {
		fmt.Println(foo)
	}

	// Since Go is statically typed, and cache values can be anything, type
	// assertion is needed when values are being passed to functions that don't
	// take arbitrary types, (i.e. interface{}). The simplest way to do this for
	// values which will only be used once--e.g. for passing to another
	// function--is:
	//由于Go是静态类型的，并且缓存值可以是任何值，因此当将值传递给不采用任意类型的函数（即interface {}）时，需要类型断言。 对于仅使用一次的值执行此操作的最简单方法-例如 传递给另一个
	foo, found := c.Get("foo")
	if found {
		MyFunction(foo.(string))
	}

	// This gets tedious if the value is used several times in the same function.
	// You might do either of the following instead:
	//如果在同一函数中多次使用该值，则将变得很乏味。 您可以执行以下任一操作：
	if x, found := c.Get("foo"); found {
		foo := x.(string)
		// ...
	}
	// or
	var foo string
	if x, found := c.Get("foo"); found {
		foo = x.(string)
	}
	// ...
	// foo can then be passed around freely as a string

	// Want performance? Store pointers!
	c.Set("foo", &MyStruct, cache.DefaultExpiration)
	if x, found := c.Get("foo"); found {
		foo := x.(*MyStruct)
			// ...
	}
}
```

### Reference

`godoc` or [http://godoc.org/github.com/patrickmn/go-cache](http://godoc.org/github.com/patrickmn/go-cache)
