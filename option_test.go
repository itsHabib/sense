package sense

import (
	"testing"
	"time"
)

func TestNew_Defaults(t *testing.T) {
	s := New()
	if s.model != "claude-sonnet-4-6" {
		t.Errorf("expected claude-sonnet-4-6, got %s", s.model)
	}
	if s.timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", s.timeout)
	}
	if s.maxRetries != 3 {
		t.Errorf("expected 3 retries, got %d", s.maxRetries)
	}
	if s.batcher != nil {
		t.Error("expected nil batcher")
	}
}

func TestNew_WithModel(t *testing.T) {
	s := New(WithModel("claude-haiku-4-5-20251001"))
	if s.model != "claude-haiku-4-5-20251001" {
		t.Errorf("expected claude-haiku-4-5-20251001, got %s", s.model)
	}
}

func TestNew_WithTimeout(t *testing.T) {
	s := New(WithTimeout(10 * time.Second))
	if s.timeout != 10*time.Second {
		t.Errorf("expected 10s, got %v", s.timeout)
	}
}

func TestNew_WithTimeoutDisabled(t *testing.T) {
	s := New(WithTimeout(-1))
	if s.timeout != 0 {
		t.Errorf("expected 0 (disabled), got %v", s.timeout)
	}
}

func TestNew_WithRetries(t *testing.T) {
	s := New(WithRetries(5))
	if s.maxRetries != 5 {
		t.Errorf("expected 5, got %d", s.maxRetries)
	}
}

func TestNew_WithRetriesDisabled(t *testing.T) {
	s := New(WithRetries(-1))
	if s.maxRetries != 0 {
		t.Errorf("expected 0 (disabled), got %d", s.maxRetries)
	}
}

func TestNew_WithAPIKey(t *testing.T) {
	s := New(WithAPIKey("sk-test-123"))
	// Key flows through to client. Verify session was created
	// (key is in the claudeClient, not directly on Session).
	if s.client == nil {
		t.Error("expected non-nil client")
	}
}

func TestNew_WithMemoryCache(t *testing.T) {
	s := New(WithMemoryCache())
	if s.cache == nil {
		t.Error("expected cache to be set")
	}
}

func TestNew_WithBatch(t *testing.T) {
	s := New(WithBatch(50, 2*time.Second))
	if s.batcher == nil {
		t.Fatal("expected batcher to be set")
	}
	if _, ok := s.client.(*batchCaller); !ok {
		t.Errorf("expected *batchCaller, got %T", s.client)
	}
	s.Close()
}

func TestNew_MultipleOptions(t *testing.T) {
	s := New(
		WithModel("claude-opus-4-6"),
		WithTimeout(60*time.Second),
		WithRetries(5),
	)
	if s.model != "claude-opus-4-6" {
		t.Errorf("expected claude-opus-4-6, got %s", s.model)
	}
	if s.timeout != 60*time.Second {
		t.Errorf("expected 60s, got %v", s.timeout)
	}
	if s.maxRetries != 5 {
		t.Errorf("expected 5, got %d", s.maxRetries)
	}
}
