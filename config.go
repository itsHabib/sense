package sense

import (
	"os"
	"time"
)

// Config holds configuration for a Session.
type Config struct {
	// APIKey for Claude. Default: $ANTHROPIC_API_KEY
	APIKey string

	// Model for evaluations. Default: "claude-sonnet-4-6"
	Model string

	// Timeout per API call. Default: 30s.
	// Set to -1 to explicitly disable timeouts.
	Timeout time.Duration

	// MaxRetries on transient failures. Default: 3.
	// Set to -1 to explicitly disable retries.
	MaxRetries int

	// Cache for response caching. Default: nil (no caching)
	Cache Cache

	// Batch enables request batching. Default: nil (individual calls).
	// When set, requests are collected and submitted as a single batch API call.
	// 50% cost reduction, same caller interface.
	Batch *BatchConfig
}

// Session holds a configured client for making evaluations.
// Create one with NewSession and pass it to your tests.
type Session struct {
	client     caller
	model      string
	timeout    time.Duration
	maxRetries int
	cache      Cache
	batcher    *batcher
}

// NewSession creates a Session from the given config.
// If Batch is set, requests are collected and submitted as a single batch API call.
func NewSession(cfg Config) *Session {
	s := &Session{
		model:      cfg.Model,
		timeout:    cfg.Timeout,
		maxRetries: cfg.MaxRetries,
		cache:      cfg.Cache,
	}

	if s.model == "" {
		s.model = "claude-sonnet-4-6"
	}
	if s.timeout == 0 {
		s.timeout = 30 * time.Second
	}
	if cfg.Timeout < 0 {
		s.timeout = 0
	}
	if s.maxRetries == 0 {
		s.maxRetries = 3
	}
	if cfg.MaxRetries < 0 {
		s.maxRetries = 0
	}

	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	if cfg.Batch != nil {
		b := newBatcher(*cfg.Batch, apiKey)
		s.batcher = b
		s.client = &batchCaller{batcher: b}
	} else {
		s.client = newClaudeClient(apiKey, s.maxRetries)
	}

	return s
}

// Close shuts down the session. If batching is enabled, it flushes
// any remaining requests before returning.
func (s *Session) Close() {
	if s.batcher != nil {
		s.batcher.close()
	}
}

func (s *Session) getModel() string {
	if m := os.Getenv("SENSE_MODEL"); m != "" {
		return m
	}
	return s.model
}

func shouldSkip() bool {
	return os.Getenv("SENSE_SKIP") == "1"
}
