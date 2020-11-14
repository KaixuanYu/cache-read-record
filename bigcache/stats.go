package bigcache

// Stats stores cache statistics
// Stats 储存 cache 统计数据
type Stats struct {
	// Hits is a number of successfully found keys
	// 命中key的次数
	Hits int64 `json:"hits"`
	// Misses is a number of not found keys
	// 未命中key的次数
	Misses int64 `json:"misses"`
	// DelHits is a number of successfully deleted keys
	// 成功删除key的次数
	DelHits int64 `json:"delete_hits"`
	// DelMisses is a number of not deleted keys
	// 删除key但是key不存在的次数
	DelMisses int64 `json:"delete_misses"`
	// Collisions is a number of happened key-collisions
	// key 冲突的次数
	Collisions int64 `json:"collisions"`
}
