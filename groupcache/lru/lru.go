/*
Copyright 2013 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package lru implements an LRU cache.
package lru

import "container/list"

// Cache is an LRU cache. It is not safe for concurrent access.
// Cache 是一个 LRU cache。 并发访问不安全
type Cache struct {
	// MaxEntries is the maximum number of cache entries before
	// an item is evicted. Zero means no limit.
	// MaxEntries 使用 cache 找那个 entries 的最大数量。Zero 代表不限制
	MaxEntries int

	// OnEvicted optionally specifies a callback function to be
	// executed when an entry is purged from the cache.
	// OnEvicted（可选）指定从缓存中清除条目时要执行的回调函数。
	OnEvicted func(key Key, value interface{})

	ll    *list.List // 一个双向链表
	cache map[interface{}]*list.Element
}

// A Key may be any value that is comparable. See http://golang.org/ref/spec#Comparison_operators
// 键可以是任何可比较的值。
type Key interface{}

type entry struct { // 一个条目
	key   Key
	value interface{}
}

// New creates a new Cache. 创建一个新的Cache
// If maxEntries is zero, the cache has no limit and it's assumed
// that eviction is done by the caller.
// 如果maxEntries为零，则缓存没有限制，并且假定逐出由调用方完成。
func New(maxEntries int) *Cache {
	return &Cache{
		MaxEntries: maxEntries,
		ll:         list.New(),
		cache:      make(map[interface{}]*list.Element),
	}
}

// Add adds a value to the cache. Add 增加一个 value 到 cache
func (c *Cache) Add(key Key, value interface{}) {
	if c.cache == nil { //cache 是 nil，就重建
		c.cache = make(map[interface{}]*list.Element)
		c.ll = list.New()
	}
	if ee, ok := c.cache[key]; ok { //如果找到了，将该值移动到list头部，然后更新value
		c.ll.MoveToFront(ee)
		ee.Value.(*entry).value = value
		return
	}
	// 原来没有该key，新增
	// 1. 放在list头部 2. 放进c.cache中
	ele := c.ll.PushFront(&entry{key, value})
	c.cache[key] = ele
	if c.MaxEntries != 0 && c.ll.Len() > c.MaxEntries {
		//如果长度大于最大个数，就删除list尾部的。list和map的都会删，然后调用回调
		c.RemoveOldest()
	}
}

// Get looks up a key's value from the cache.
// Get从缓存中查找键的值。
func (c *Cache) Get(key Key) (value interface{}, ok bool) {
	if c.cache == nil {
		return
	}
	if ele, hit := c.cache[key]; hit {
		c.ll.MoveToFront(ele)
		return ele.Value.(*entry).value, true
	}
	return
}

// Remove removes the provided key from the cache.
// 删除将从缓存中删除提供的key。
func (c *Cache) Remove(key Key) {
	if c.cache == nil {
		return
	}
	if ele, hit := c.cache[key]; hit {
		c.removeElement(ele)
	}
}

// RemoveOldest removes the oldest item from the cache.
// RemoveOldest 从 cache 中移除 旧的item
func (c *Cache) RemoveOldest() {
	if c.cache == nil {
		return
	}
	ele := c.ll.Back() //从list尾拿出来
	if ele != nil {
		c.removeElement(ele)
	}
}

func (c *Cache) removeElement(e *list.Element) {
	c.ll.Remove(e) //list删除
	kv := e.Value.(*entry)
	delete(c.cache, kv.key) // map中删除
	if c.OnEvicted != nil { //删除回调
		c.OnEvicted(kv.key, kv.value)
	}
}

// Len returns the number of items in the cache.
// Len 返回 cache 中 items 的长度
func (c *Cache) Len() int {
	if c.cache == nil {
		return 0
	}
	return c.ll.Len()
}

// Clear purges all stored items from the cache.
// 清除将从高速缓存中清除所有存储的项目。
func (c *Cache) Clear() {
	if c.OnEvicted != nil {
		for _, e := range c.cache {
			kv := e.Value.(*entry)
			c.OnEvicted(kv.key, kv.value)
		}
	}
	c.ll = nil
	c.cache = nil
}
