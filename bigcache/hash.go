package bigcache

// Hasher is responsible for generating unsigned, 64 bit hash of provided string. Hasher should minimize collisions
// (generating same hash for different strings) and while performance is also important fast functions are preferable (i.e.
// you can use FarmHash family).
// Hasher 负责生成 指定的string的无符号64位的hash值。Hasher应该最大程度的减少冲突，虽然性能也重要，但是快更可取（例如，您可以使用FarmHash系列）
type Hasher interface {
	Sum64(string) uint64
}
