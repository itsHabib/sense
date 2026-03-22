package sense

// Cache stores and retrieves API responses.
type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, data []byte)
}

// FileCache creates a disk-based cache at the given directory.
// Responses are stored as JSON files named by content hash.
// Commit the directory to your repo for free CI runs.
func FileCache(dir string) Cache {
	return &fileCache{dir: dir}
}

// MemoryCache creates an in-memory cache. Useful for test isolation.
func MemoryCache() Cache {
	return &memoryCache{store: make(map[string][]byte)}
}

type fileCache struct {
	dir string
}

func (c *fileCache) Get(key string) ([]byte, bool) {
	// TODO: Phase 2 - implement file-based cache
	return nil, false
}

func (c *fileCache) Set(key string, data []byte) {
	// TODO: Phase 2 - implement file-based cache
}

type memoryCache struct {
	store map[string][]byte
}

func (c *memoryCache) Get(key string) ([]byte, bool) {
	data, ok := c.store[key]
	return data, ok
}

func (c *memoryCache) Set(key string, data []byte) {
	c.store[key] = data
}
