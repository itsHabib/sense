package sense

import "sync"

// Cache stores and retrieves API responses by content-addressed key.
type Cache interface {
	// Get retrieves a cached response. Returns the data and true on hit,
	// nil and false on miss.
	Get(key string) ([]byte, bool)

	// Set stores a response in the cache.
	Set(key string, data []byte)
}

// FileCache creates a disk-based cache at the given directory.
// Not yet implemented — returns a no-op cache.
// TODO: implement in Phase 2.
func FileCache(dir string) Cache {
	return &fileCache{dir: dir}
}

// MemoryCache creates an in-memory cache safe for concurrent use.
func MemoryCache() Cache {
	return &memoryCache{store: make(map[string][]byte)}
}

type fileCache struct {
	dir string
}

func (c *fileCache) Get(_ string) ([]byte, bool) {
	return nil, false
}

func (c *fileCache) Set(_ string, _ []byte) {
}

type memoryCache struct {
	mu    sync.RWMutex
	store map[string][]byte
}

func (c *memoryCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	data, ok := c.store[key]
	return data, ok
}

func (c *memoryCache) Set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = data
}
