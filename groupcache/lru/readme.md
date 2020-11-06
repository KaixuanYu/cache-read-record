# lru

- 一个map，还不带锁（多线程不安全）
- 用一个list（双向链表）来做lru，每次新建更新检索都将该key：value对放移动到list头不
- 新增一个key：value对，如果超过最大长度限制，就将最old的key：value对删除，然后新增新的
- 对外提供基于key的Delete和基于old的Delete