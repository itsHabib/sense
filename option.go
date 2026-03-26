package sense

import (
	"os"
	"time"
)

// Option configures a Session. Pass options to New or ForTest.
type Option func(*sessionConfig)

// sessionConfig accumulates option values before building a Session.
type sessionConfig struct {
	apiKey     string
	model      string
	timeout    time.Duration
	maxRetries int
	cache      Cache
	batch      *BatchConfig

	// sentinels: track whether the caller explicitly set timeout/retries,
	// so we can distinguish "not set" from "set to zero".
	timeoutSet    bool
	maxRetriesSet bool
}

// WithModel sets the Claude model for evaluations.
func WithModel(model string) Option {
	return func(c *sessionConfig) {
		c.model = model
	}
}

// WithTimeout sets the per-call timeout. Set to -1 to disable timeouts.
func WithTimeout(d time.Duration) Option {
	return func(c *sessionConfig) {
		c.timeout = d
		c.timeoutSet = true
	}
}

// WithRetries sets the number of retry attempts on transient failures.
// Set to -1 to disable retries.
func WithRetries(n int) Option {
	return func(c *sessionConfig) {
		c.maxRetries = n
		c.maxRetriesSet = true
	}
}

// WithBatch enables request batching. Requests are collected and submitted
// as a single batch API call (50% cost reduction). Session.Close must be
// called to flush pending requests.
func WithBatch(maxSize int, maxWait time.Duration) Option {
	return func(c *sessionConfig) {
		c.batch = &BatchConfig{MaxSize: maxSize, MaxWait: maxWait}
	}
}

// WithMemoryCache enables an in-memory response cache for the session.
// Identical requests return the cached response instead of hitting the API.
// The cache lives and dies with the session.
func WithMemoryCache() Option {
	return func(c *sessionConfig) {
		c.cache = MemoryCache()
	}
}

// WithAPIKey sets the Anthropic API key. Default: $ANTHROPIC_API_KEY.
func WithAPIKey(key string) Option {
	return func(c *sessionConfig) {
		c.apiKey = key
	}
}

func applyDefaults(cfg *sessionConfig) {
	if cfg.model == "" {
		cfg.model = "claude-sonnet-4-6"
	}
	if !cfg.timeoutSet {
		cfg.timeout = 30 * time.Second
	}
	if cfg.timeoutSet && cfg.timeout < 0 {
		cfg.timeout = 0
	}
	if !cfg.maxRetriesSet {
		cfg.maxRetries = 3
	}
	if cfg.maxRetriesSet && cfg.maxRetries < 0 {
		cfg.maxRetries = 0
	}
	if cfg.apiKey == "" {
		cfg.apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
}

func buildSession(cfg *sessionConfig) *Session {
	s := &Session{
		model:      cfg.model,
		timeout:    cfg.timeout,
		maxRetries: cfg.maxRetries,
		cache:      cfg.cache,
	}

	var c caller
	if cfg.batch != nil {
		b := newBatcher(*cfg.batch, cfg.apiKey)
		s.batcher = b
		c = &batchCaller{batcher: b}
	} else {
		c = newClaudeClient(cfg.apiKey, s.maxRetries)
	}

	if cfg.cache != nil {
		c = &cachedCaller{inner: c, cache: cfg.cache}
	}

	s.client = c
	return s
}
