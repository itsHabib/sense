package sense

import (
	"log/slog"
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
	context    string

	minConfidence    float64
	minConfidenceSet bool

	// sentinels: track whether the caller explicitly set timeout/retries,
	// so we can distinguish "not set" from "set to zero".
	timeoutSet    bool
	maxRetriesSet bool

	logger *slog.Logger
	hook   func(Event)
}

// WithModel sets the Claude model for evaluations.
func WithModel(model string) Option {
	return func(c *sessionConfig) {
		c.model = model
	}
}

// WithTimeout sets the per-call timeout. Set to -1 or 0 to disable timeouts.
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

// WithMinConfidence sets a session-level minimum confidence threshold.
// Checks that pass Claude's judgment but fall below this threshold are
// treated as failures. Per-call MinConfidence overrides this default.
func WithMinConfidence(threshold float64) Option {
	return func(c *sessionConfig) {
		c.minConfidence = threshold
		c.minConfidenceSet = true
	}
}

// WithContext sets a session-level context string that is prepended to
// every evaluation and extraction call. Per-call Context appends to it.
func WithContext(ctx string) Option {
	return func(c *sessionConfig) {
		c.context = ctx
	}
}

// WithLogger sets a structured logger for the session. When set, sense
// logs API calls, latencies, token usage, and errors.
func WithLogger(l *slog.Logger) Option {
	return func(c *sessionConfig) {
		c.logger = l
	}
}

// WithHook sets a callback invoked after every API call with event details.
func WithHook(fn func(Event)) Option {
	return func(c *sessionConfig) {
		c.hook = fn
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
		model:         cfg.model,
		timeout:       cfg.timeout,
		maxRetries:    cfg.maxRetries,
		cache:         cfg.cache,
		context:       cfg.context,
		minConfidence: cfg.minConfidence,
		logger:        cfg.logger,
		hook:          cfg.hook,
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
		c = &cachedCaller{inner: c, cache: cfg.cache, logger: cfg.logger}
	}

	s.client = c
	return s
}
