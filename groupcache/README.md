# groupcache

## Summary 概要

groupcache is a distributed caching and cache-filling library, intended as a
replacement for a pool of memcached nodes in many cases.

groupcache 是一个分布式缓存和缓存填充库，在许多情况下可用来替代memcached节点池。

For API docs and examples, see http://godoc.org/github.com/golang/groupcache

有关API文档和示例，请参见 http://godoc.org/github.com/golang/groupcache

## Comparison to memcached [与memcached的比较]

### **Like memcached**, groupcache: [与memcached相似的地方]

 * shards by key to select which peer is responsible for that key

 * 对key进行分片

### **Unlike memcached**, groupcache: [与memcached不相似的地方]

 * does not require running a separate set of servers, thus massively
   reducing deployment/configuration pain.  groupcache is a client
   library as well as a server.  It connects to its own peers, forming
   a distributed cache.
   
 * [翻译] 不需要运行单独的服务集群，不用部署了很省事啊。groupcache是一个客户端库，
   连接到其他节点来形成分布式cache。

 * comes with a cache filling mechanism.  Whereas memcached just says
   "Sorry, cache miss", often resulting in a thundering herd of
   database (or whatever) loads from an unbounded number of clients
   (which has resulted in several fun outages), groupcache coordinates
   cache fills such that only one load in one process of an entire
   replicated set of processes populates the cache, then multiplexes
   the loaded value to all callers.
   
 * [翻译] 带有缓存填充机制。 

 * does not support versioned values.  If key "foo" is value "bar",
   key "foo" must always be "bar".  There are neither cache expiration
   times, nor explicit cache evictions.  Thus there is also no CAS,
   nor Increment/Decrement.  This also means that groupcache....

 * [翻译]不支持版本值。 如果键“ foo”为值“ bar”，则键“ foo”必须始终为“ bar”。 既没有缓存过期时间，也没有明确的缓存逐出。 因此，也没有CAS，也没有增量/减量。 这也意味着groupcache...。

 * ... supports automatic mirroring of super-hot items to multiple
   processes.  This prevents memcached hot spotting where a machine's
   CPU and/or NIC are overloaded by very popular keys/values.

 * [翻译]... supports automatic mirroring of super-hot items to multiple processes. This prevents memcached hot spotting where a machine's CPU and/or NIC are overloaded by very popular keys/values.
 
 * is currently only available for Go.  It's very unlikely that I
   (bradfitz@) will port the code to any other language.

 * [翻译]当前仅可用于Go。我（bradfitz@）不太可能将代码移植到任何其他语言。

## Loading process [loading 过程]

In a nutshell, a groupcache lookup of **Get("foo")** looks like:

简而言之，groupcache 的 ** Get("foo") ** 查找如下：

(On machine #5 of a set of N machines running the same code)

（在运行相同代码的一组N台机器中的5号机器上）

 1. Is the value of "foo" in local memory because it's super hot?  If so, use it.

    本地存储器中的“ foo”值是否因为其过热而存在吗？ 如果是这样，请使用它。
    
 2. Is the value of "foo" in local memory because peer #5 (the current
    peer) is the owner of it?  If so, use it.
    
    本地存储器中的“foo”值是否因为是它的拥有者而存在吗？ 如果是这样，请使用它。

 3. Amongst all the peers in my set of N, am I the owner of the key
    "foo"?  (e.g. does it consistent hash to 5?)  If so, load it.  If
    other callers come in, via the same process or via RPC requests
    from peers, they block waiting for the load to finish and get the
    same answer.  If not, RPC to the peer that's the owner and get
    the answer.  If the RPC fails, just load it locally (still with
    local dup suppression).
    
    在我的N组中，我是键“foo”的所有者吗？（例如，哈希值是否一致为5？）如果是，请加载它。
    如果其他调用方通过同一进程或来自对等方的RPC请求进来，它们将阻塞等待加载完成并获得相同的答案。、
    如果没有，RPC到对等的所有者并得到答案。如果RPC失败，只需在本地加载它（仍然使用本地dup抑制）。

## Presentations [演示文稿]

See http://talks.golang.org/2013/oscon-dl.slide