# lru

- 一个map，还不带锁（多线程不安全）
- 用一个list（双向链表）来做lru，每次新建更新检索都将该key：value对放移动到list头不
- 新增一个key：value对，如果超过最大长度限制，就将最old的key：value对删除，然后新增新的
- 对外提供基于key的Delete和基于old的Delete
- 删除old的item在队尾删除

## group 对 lru.cache 的使用

group 对 lru.cache 进行了一次封装。做了两件事:
1. 加了锁
2. 加了统计信息，包括cache数据总大小，get次数，驱逐次数，缓存命中次数。