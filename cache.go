package sense

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

// Cache stores and retrieves API responses by content-addressed key.
type Cache interface {
	// Get retrieves a cached response. Returns the data and true on hit,
	// nil and false on miss.
	Get(key string) ([]byte, bool)

	// Set stores a response in the cache.
	Set(key string, data []byte)
}

// MemoryCache creates an in-memory cache safe for concurrent use.
func MemoryCache() Cache {
	return &memoryCache{store: make(map[string][]byte)}
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

// cachedCaller wraps a caller with a cache layer. On cache hit, the API
// call is skipped entirely. On miss, the response is stored after a
// successful call.
type cachedCaller struct {
	inner caller
	cache Cache
}

// cacheEntry is the serialized form stored in the cache.
type cacheEntry struct {
	Raw   json.RawMessage `json:"raw"`
	Usage *Usage          `json:"usage,omitempty"`
}

func (c *cachedCaller) call(ctx context.Context, req *callRequest) (json.RawMessage, *Usage, error) {
	key := cacheKey(req)

	if data, ok := c.cache.Get(key); ok {
		var entry cacheEntry
		if err := json.Unmarshal(data, &entry); err == nil {
			return entry.Raw, entry.Usage, nil
		}
	}

	raw, usage, err := c.inner.call(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	entry := cacheEntry{Raw: raw, Usage: usage}
	if data, err := json.Marshal(entry); err == nil {
		c.cache.Set(key, data)
	}

	return raw, usage, nil
}

func cacheKey(req *callRequest) string {
	h := sha256.New()
	for _, s := range []string{req.model, req.systemPrompt, req.userMessage, req.toolName} {
		h.Write([]byte(s))
		h.Write([]byte{'\n'})
	}
	schema, _ := json.Marshal(req.toolSchema)
	h.Write(schema)
	return hex.EncodeToString(h.Sum(nil))
}
