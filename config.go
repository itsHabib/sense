package sense

import (
	"os"
	"sync"
	"time"
)

// Config holds global configuration for the sense package.
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
}

var (
	mu           sync.RWMutex
	globalConfig = Config{
		Model:      "claude-sonnet-4-6",
		Timeout:    30 * time.Second,
		MaxRetries: 3,
	}

	clientOnce   sync.Once
	globalClient *claudeClient
)

// Configure sets global defaults. Call in TestMain or init.
// Fields are applied as overrides — only non-zero values are set.
// Use -1 for Timeout or MaxRetries to explicitly set zero behavior.
func Configure(cfg Config) {
	mu.Lock()
	defer mu.Unlock()

	if cfg.APIKey != "" {
		globalConfig.APIKey = cfg.APIKey
	}
	if cfg.Model != "" {
		globalConfig.Model = cfg.Model
	}
	if cfg.Timeout != 0 {
		if cfg.Timeout < 0 {
			globalConfig.Timeout = 0
		} else {
			globalConfig.Timeout = cfg.Timeout
		}
	}
	if cfg.MaxRetries != 0 {
		if cfg.MaxRetries < 0 {
			globalConfig.MaxRetries = 0
		} else {
			globalConfig.MaxRetries = cfg.MaxRetries
		}
	}
	if cfg.Cache != nil {
		globalConfig.Cache = cfg.Cache
	}

	// Reset client so it picks up new config on next use.
	clientOnce = sync.Once{}
	globalClient = nil
}

func getAPIKey() string {
	mu.RLock()
	defer mu.RUnlock()

	if globalConfig.APIKey != "" {
		return globalConfig.APIKey
	}
	return os.Getenv("ANTHROPIC_API_KEY")
}

func getModel() string {
	if m := os.Getenv("SENSE_MODEL"); m != "" {
		return m
	}
	mu.RLock()
	defer mu.RUnlock()
	return globalConfig.Model
}

func getTimeout() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	return globalConfig.Timeout
}

func getMaxRetries() int {
	mu.RLock()
	defer mu.RUnlock()
	return globalConfig.MaxRetries
}

func shouldSkip() bool {
	return os.Getenv("SENSE_SKIP") == "1"
}

func getClient() *claudeClient {
	clientOnce.Do(func() {
		globalClient = newClaudeClient(getAPIKey())
	})
	return globalClient
}
