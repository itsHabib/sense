package sense

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// SessionUsage is a snapshot of accumulated token usage across a session.
type SessionUsage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	Calls        int64
}

// String returns a human-readable summary of token usage.
func (u SessionUsage) String() string {
	return fmt.Sprintf("sense: %d calls, %d input tokens, %d output tokens",
		u.Calls, u.InputTokens, u.OutputTokens)
}

// Session holds a configured client for making evaluations.
// Create one with New or ForTest.
type Session struct {
	client     caller
	model      string
	timeout    time.Duration
	maxRetries int
	cache      Cache
	batcher    *batcher

	inputTokens  atomic.Int64
	outputTokens atomic.Int64
	callCount    atomic.Int64
}

// New creates a Session with functional options.
//
//	s := sense.New()                                        // defaults
//	s := sense.New(sense.WithModel("claude-haiku-4-5-20251001"))  // custom model
//	s := sense.New(sense.WithBatch(50, 2*time.Second))     // batching (requires defer s.Close())
func New(opts ...Option) *Session {
	cfg := &sessionConfig{}
	for _, o := range opts {
		o(cfg)
	}
	applyDefaults(cfg)
	return buildSession(cfg)
}

// Close shuts down the session. If batching is enabled, it flushes
// any remaining requests before returning.
func (s *Session) Close() {
	if s.batcher != nil {
		s.batcher.close()
	}
}

// recordUsage accumulates token usage from a single call.
// Safe for concurrent callers.
func (s *Session) recordUsage(u *Usage) {
	if u == nil {
		return
	}
	s.inputTokens.Add(int64(u.InputTokens))
	s.outputTokens.Add(int64(u.OutputTokens))
	s.callCount.Add(1)
}

// Usage returns a snapshot of accumulated token usage across the session.
func (s *Session) Usage() SessionUsage {
	input := s.inputTokens.Load()
	output := s.outputTokens.Load()
	return SessionUsage{
		InputTokens:  input,
		OutputTokens: output,
		TotalTokens:  input + output,
		Calls:        s.callCount.Load(),
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
