package sense

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCachedCaller_HitSkipsInner(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"pass": true}`),
		usage:    &Usage{InputTokens: 100, OutputTokens: 50},
	}
	cache := MemoryCache()
	cc := &cachedCaller{inner: mock, cache: cache}

	req := &callRequest{
		systemPrompt: "sys",
		userMessage:  "hello",
		toolName:     "submit",
		model:        "claude-sonnet-4-6",
	}

	// First call — miss, hits inner.
	raw1, usage1, err := cc.call(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 inner call, got %d", mock.calls)
	}

	// Second call — hit, skips inner.
	raw2, usage2, err := cc.call(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.calls != 1 {
		t.Errorf("expected inner call count to stay 1, got %d", mock.calls)
	}
	var v1, v2 map[string]any
	json.Unmarshal(raw1, &v1) //nolint:errcheck // test comparison
	json.Unmarshal(raw2, &v2) //nolint:errcheck // test comparison
	if v1["pass"] != v2["pass"] {
		t.Errorf("expected same response, got %s vs %s", raw1, raw2)
	}
	if usage1.InputTokens != usage2.InputTokens {
		t.Errorf("expected same usage, got %d vs %d", usage1.InputTokens, usage2.InputTokens)
	}
}

func TestCachedCaller_DifferentRequestsMiss(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"ok": true}`),
		usage:    &Usage{InputTokens: 10, OutputTokens: 5},
	}
	cache := MemoryCache()
	cc := &cachedCaller{inner: mock, cache: cache}

	req1 := &callRequest{systemPrompt: "sys", userMessage: "hello", toolName: "t", model: "m"}
	req2 := &callRequest{systemPrompt: "sys", userMessage: "world", toolName: "t", model: "m"}

	_, _, _ = cc.call(context.Background(), req1)
	_, _, _ = cc.call(context.Background(), req2)

	if mock.calls != 2 {
		t.Errorf("expected 2 inner calls for different requests, got %d", mock.calls)
	}
}

func TestCachedCaller_ErrorNotCached(t *testing.T) {
	callCount := 0
	mock := &mockCaller{}
	cache := MemoryCache()
	cc := &cachedCaller{inner: mock, cache: cache}

	// First call fails.
	mock.err = ErrRateLimit
	req := &callRequest{systemPrompt: "sys", userMessage: "msg", toolName: "t", model: "m"}
	_, _, err := cc.call(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	callCount++

	// Second call succeeds — should hit inner again since error wasn't cached.
	mock.err = nil
	mock.response = json.RawMessage(`{"ok": true}`)
	mock.usage = &Usage{InputTokens: 10, OutputTokens: 5}
	_, _, err = cc.call(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	callCount++

	if mock.calls != callCount {
		t.Errorf("expected %d inner calls, got %d", callCount, mock.calls)
	}
}

func TestCachedCaller_SessionIntegration(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"device": "x", "volume_id": "y", "message": "z"}`),
		usage:    &Usage{InputTokens: 100, OutputTokens: 50},
	}

	s := &Session{
		client:     &cachedCaller{inner: mock, cache: MemoryCache()},
		model:      "claude-sonnet-4-6",
		timeout:    0,
		maxRetries: 3,
	}

	// Two identical extractions — second should hit cache.
	_, _ = newExtractBuilder[mountError](s, "same text").Run()
	_, _ = newExtractBuilder[mountError](s, "same text").Run()

	if mock.calls != 1 {
		t.Errorf("expected 1 API call (second should be cached), got %d", mock.calls)
	}

	// Usage is recorded for both (cache returns usage too).
	u := s.Usage()
	if u.Calls != 2 {
		t.Errorf("expected 2 recorded calls, got %d", u.Calls)
	}
}

func TestCacheKey_Deterministic(t *testing.T) {
	req := &callRequest{
		systemPrompt: "sys",
		userMessage:  "msg",
		toolName:     "tool",
		model:        "model",
	}

	k1 := cacheKey(req)
	k2 := cacheKey(req)
	if k1 != k2 {
		t.Errorf("expected deterministic key, got %s vs %s", k1, k2)
	}
}

func TestCacheKey_VariesByField(t *testing.T) {
	base := &callRequest{
		systemPrompt: "sys",
		userMessage:  "msg",
		toolName:     "tool",
		model:        "model",
	}

	variants := []*callRequest{
		{systemPrompt: "other", userMessage: "msg", toolName: "tool", model: "model"},
		{systemPrompt: "sys", userMessage: "other", toolName: "tool", model: "model"},
		{systemPrompt: "sys", userMessage: "msg", toolName: "other", model: "model"},
		{systemPrompt: "sys", userMessage: "msg", toolName: "tool", model: "other"},
	}

	baseKey := cacheKey(base)
	for i, v := range variants {
		k := cacheKey(v)
		if k == baseKey {
			t.Errorf("variant %d produced same key as base", i)
		}
	}
}
