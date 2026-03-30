package sense

import (
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

// SessionUsage is a snapshot of accumulated token usage across a session.
type SessionUsage struct {
	InputTokens   int64
	OutputTokens  int64
	TotalTokens   int64
	Calls         int64
	EstimatedCost float64
}

// String returns a human-readable summary of token usage.
func (u SessionUsage) String() string {
	if u.EstimatedCost > 0 {
		return fmt.Sprintf("sense: %d calls, %d input tokens, %d output tokens (~$%.4f)",
			u.Calls, u.InputTokens, u.OutputTokens, u.EstimatedCost)
	}
	return fmt.Sprintf("sense: %d calls, %d input tokens, %d output tokens",
		u.Calls, u.InputTokens, u.OutputTokens)
}

// Dollars converts a dollar amount for use with WithMaxCost.
func Dollars(n float64) float64 { return n }

// Model pricing per million tokens (input, output).
var modelPricing = map[string][2]float64{
	"claude-sonnet-4-6":       {3.0, 15.0},
	"claude-opus-4-6":         {15.0, 75.0},
	"claude-haiku-4-5-20251001": {0.80, 4.0},
}

func estimateCost(model string, input, output int64) float64 {
	prices, ok := modelPricing[model]
	if !ok {
		prices = modelPricing["claude-sonnet-4-6"] // default
	}
	return float64(input)/1_000_000*prices[0] + float64(output)/1_000_000*prices[1]
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
	context    string

	minConfidence float64

	logger *slog.Logger
	hook   func(Event)

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
		InputTokens:   input,
		OutputTokens:  output,
		TotalTokens:   input + output,
		Calls:         s.callCount.Load(),
		EstimatedCost: estimateCost(s.model, input, output),
	}
}

func (s *Session) getModel() string {
	if m := os.Getenv("SENSE_MODEL"); m != "" {
		return m
	}
	return s.model
}

// emit fires observability hooks and logs.
func (s *Session) emit(e Event) {
	if s.hook != nil {
		s.hook(e)
	}
	if s.logger != nil {
		attrs := []any{
			slog.String("op", e.Op),
			slog.String("model", e.Model),
			slog.Duration("duration", e.Duration),
			slog.Int("tokens", e.Tokens),
		}
		if e.Err != nil {
			attrs = append(attrs, slog.String("error", e.Err.Error()))
			s.logger.Error("sense call failed", attrs...)
		} else {
			s.logger.Info("sense call", attrs...)
		}
	}
}

func shouldSkip() bool {
	return os.Getenv("SENSE_SKIP") == "1"
}
