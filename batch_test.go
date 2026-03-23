package sense

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBatchCaller_ImplementsCaller(_ *testing.T) {
	var _ caller = (*batchCaller)(nil)
}

func TestBatchConfig_InConfig(t *testing.T) {
	cfg := Config{
		Batch: &BatchConfig{
			MaxSize: 10,
			MaxWait: 500 * time.Millisecond,
		},
	}
	if cfg.Batch.MaxSize != 10 {
		t.Errorf("expected MaxSize=10, got %d", cfg.Batch.MaxSize)
	}
	if cfg.Batch.MaxWait != 500*time.Millisecond {
		t.Errorf("expected MaxWait=500ms, got %v", cfg.Batch.MaxWait)
	}
}

func TestBatchConfig_NilByDefault(t *testing.T) {
	cfg := Config{}
	if cfg.Batch != nil {
		t.Error("expected Batch to be nil by default")
	}
}

func TestBatchResult_Fields(t *testing.T) {
	r := batchResult{
		raw:   json.RawMessage(`{"pass": true}`),
		usage: &Usage{InputTokens: 100, OutputTokens: 50},
	}
	if r.err != nil {
		t.Errorf("expected no error, got %v", r.err)
	}
	if r.usage.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", r.usage.InputTokens)
	}
}

func TestBatchResult_Error(t *testing.T) {
	r := batchResult{err: ErrNoToolCall}
	if !errors.Is(r.err, ErrNoToolCall) {
		t.Errorf("expected ErrNoToolCall, got %v", r.err)
	}
}

func TestPendingRequest_ChannelBuffered(t *testing.T) {
	ch := make(chan batchResult, 1)
	p := pendingRequest{
		id:   "req-1",
		req:  &callRequest{toolName: "submit_evaluation"},
		resp: ch,
	}

	p.resp <- batchResult{raw: json.RawMessage(`{}`), usage: &Usage{}}

	result := <-p.resp
	if result.err != nil {
		t.Errorf("expected no error, got %v", result.err)
	}
}

func TestBatcher_IDGeneration(t *testing.T) {
	b := &batcher{
		inbox: make(chan pendingRequest),
		stop:  make(chan struct{}),
	}

	id1 := b.idCounter.Add(1)
	id2 := b.idCounter.Add(1)
	if id1 != 1 || id2 != 2 {
		t.Errorf("expected sequential IDs 1,2 — got %d,%d", id1, id2)
	}
}

func TestBatcher_IDsUnique(t *testing.T) {
	b := &batcher{
		inbox: make(chan pendingRequest),
		stop:  make(chan struct{}),
	}

	const n = 100
	ids := make(chan int64, n)
	var wg sync.WaitGroup
	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()
			ids <- b.idCounter.Add(1)
		}()
	}

	wg.Wait()
	close(ids)

	seen := make(map[int64]bool, n)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate ID: %d", id)
		}
		seen[id] = true
	}

	if len(seen) != n {
		t.Errorf("expected %d unique IDs, got %d", n, len(seen))
	}
}

func TestFanOutError_AllCallersGetError(t *testing.T) {
	const n = 5
	pending := make([]pendingRequest, n)
	for i := range n {
		pending[i] = pendingRequest{
			id:   "req-" + string(rune('0'+i)),
			resp: make(chan batchResult, 1),
		}
	}

	testErr := ErrNoToolCall
	fanOutError(pending, testErr)

	for i, p := range pending {
		result := <-p.resp
		if !errors.Is(result.err, testErr) {
			t.Errorf("caller %d: expected %v, got %v", i, testErr, result.err)
		}
	}
}

func TestBuildBatchParams_CorrectStructure(t *testing.T) {
	pending := []pendingRequest{
		{
			id: "req-1",
			req: &callRequest{
				systemPrompt: "You are a judge",
				userMessage:  "evaluate this",
				toolName:     "submit_evaluation",
				toolSchema:   evalToolSchema,
				model:        "claude-sonnet-4-6",
			},
		},
		{
			id: "req-2",
			req: &callRequest{
				systemPrompt: "You compare things",
				userMessage:  "compare these",
				toolName:     "submit_comparison",
				toolSchema:   compareToolSchema,
				model:        "claude-sonnet-4-6",
			},
		},
	}

	params := buildBatchParams(pending)

	if len(params.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(params.Requests))
	}

	r1 := params.Requests[0]
	if r1.CustomID != "req-1" {
		t.Errorf("expected custom_id=req-1, got %s", r1.CustomID)
	}
	if r1.Params.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", r1.Params.Model)
	}
	if r1.Params.MaxTokens != 4096 {
		t.Errorf("expected max_tokens=4096, got %d", r1.Params.MaxTokens)
	}
	if len(r1.Params.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(r1.Params.Tools))
	}
	if r1.Params.Tools[0].OfTool.Name != "submit_evaluation" {
		t.Errorf("expected tool name submit_evaluation, got %s", r1.Params.Tools[0].OfTool.Name)
	}

	r2 := params.Requests[1]
	if r2.CustomID != "req-2" {
		t.Errorf("expected custom_id=req-2, got %s", r2.CustomID)
	}
	if r2.Params.Tools[0].OfTool.Name != "submit_comparison" {
		t.Errorf("expected tool name submit_comparison, got %s", r2.Params.Tools[0].OfTool.Name)
	}
}

func TestNewSession_BatchConfig(t *testing.T) {
	s := NewSession(Config{
		Batch: &BatchConfig{
			MaxSize: 10,
			MaxWait: 1 * time.Second,
		},
	})
	defer s.Close()

	if _, ok := s.client.(*batchCaller); !ok {
		t.Fatalf("expected *batchCaller, got %T", s.client)
	}
}

func TestNewSession_NoBatch(t *testing.T) {
	s := NewSession(Config{})
	defer s.Close()

	if _, ok := s.client.(*claudeClient); !ok {
		t.Fatalf("expected *claudeClient, got %T", s.client)
	}
}
