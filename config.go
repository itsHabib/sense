package sense

import (
	"os"
	"time"
)

// Config holds global AgentKit configuration.
type Config struct {
	// APIKey for Claude. Default: $ANTHROPIC_API_KEY
	APIKey string

	// Model for evaluations. Default: "claude-sonnet-4-6"
	Model string

	// Timeout per API call. Default: 30s
	Timeout time.Duration

	// MaxRetries on transient failures. Default: 3
	MaxRetries int

	// Cache for response caching. Default: nil (no caching)
	Cache Cache
}

var globalConfig = Config{
	Model:      "claude-sonnet-4-6",
	Timeout:    30 * time.Second,
	MaxRetries: 3,
}

var globalClient *claudeClient

// Configure sets global defaults. Call in TestMain or init.
func Configure(cfg Config) {
	if cfg.APIKey != "" {
		globalConfig.APIKey = cfg.APIKey
	}
	if cfg.Model != "" {
		globalConfig.Model = cfg.Model
	}
	if cfg.Timeout != 0 {
		globalConfig.Timeout = cfg.Timeout
	}
	if cfg.MaxRetries != 0 {
		globalConfig.MaxRetries = cfg.MaxRetries
	}
	if cfg.Cache != nil {
		globalConfig.Cache = cfg.Cache
	}
	// Reset client so it picks up new config
	globalClient = nil
}

func getAPIKey() string {
	if globalConfig.APIKey != "" {
		return globalConfig.APIKey
	}
	return os.Getenv("ANTHROPIC_API_KEY")
}

func getModel() string {
	if m := os.Getenv("SENSE_MODEL"); m != "" {
		return m
	}
	return globalConfig.Model
}

func shouldSkip() bool {
	return os.Getenv("SENSE_SKIP") == "1"
}

func getClient() *claudeClient {
	if globalClient == nil {
		globalClient = newClaudeClient(getAPIKey())
	}
	return globalClient
}
