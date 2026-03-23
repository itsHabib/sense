package sense

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
)

func TestUsage_AccumulatesAcrossEvals(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"pass": true,
			"score": 1.0,
			"checks": [{"expect": "ok", "pass": true, "confidence": 0.9, "reason": "fine"}]
		}`),
		usage: &Usage{InputTokens: 100, OutputTokens: 50},
	}
	s := testSession(mock)

	for range 3 {
		_, err := s.Eval("test").Expect("ok").Judge()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	u := s.Usage()
	if u.Calls != 3 {
		t.Errorf("expected 3 calls, got %d", u.Calls)
	}
	if u.InputTokens != 300 {
		t.Errorf("expected 300 input tokens, got %d", u.InputTokens)
	}
	if u.OutputTokens != 150 {
		t.Errorf("expected 150 output tokens, got %d", u.OutputTokens)
	}
	if u.TotalTokens != 450 {
		t.Errorf("expected 450 total tokens, got %d", u.TotalTokens)
	}
}

func TestUsage_ConcurrentSafety(t *testing.T) {
	mock := &concurrentMockCaller{
		response: json.RawMessage(`{
			"pass": true,
			"score": 1.0,
			"checks": [{"expect": "ok", "pass": true, "confidence": 0.9, "reason": "fine"}]
		}`),
		usage: &Usage{InputTokens: 10, OutputTokens: 5},
	}
	s := &Session{
		client:     mock,
		model:      "claude-sonnet-4-6",
		timeout:    0,
		maxRetries: 3,
	}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_, _ = s.Eval("test").Expect("ok").Judge()
		}()
	}
	wg.Wait()

	u := s.Usage()
	if u.Calls != goroutines {
		t.Errorf("expected %d calls, got %d", goroutines, u.Calls)
	}
	if u.InputTokens != goroutines*10 {
		t.Errorf("expected %d input tokens, got %d", goroutines*10, u.InputTokens)
	}
	if u.OutputTokens != goroutines*5 {
		t.Errorf("expected %d output tokens, got %d", goroutines*5, u.OutputTokens)
	}
}

func TestSessionUsage_String(t *testing.T) {
	u := SessionUsage{
		InputTokens:  18420,
		OutputTokens: 4210,
		TotalTokens:  22630,
		Calls:        15,
	}

	got := u.String()
	want := "sense: 15 calls, 18420 input tokens, 4210 output tokens"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestUsage_RecordsCompare(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"winner": "A",
			"score_a": 0.9,
			"score_b": 0.2,
			"criteria": [{"name": "quality", "score_a": 0.9, "score_b": 0.2, "winner": "A", "reason": "better"}],
			"reasoning": "A wins"
		}`),
		usage: &Usage{InputTokens: 200, OutputTokens: 80},
	}
	s := testSession(mock)

	_, err := s.Compare("a", "b").Criteria("quality").Judge()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u := s.Usage()
	if u.Calls != 1 {
		t.Errorf("expected 1 call, got %d", u.Calls)
	}
	if u.InputTokens != 200 {
		t.Errorf("expected 200 input tokens, got %d", u.InputTokens)
	}
	if u.OutputTokens != 80 {
		t.Errorf("expected 80 output tokens, got %d", u.OutputTokens)
	}
}

// concurrentMockCaller is a goroutine-safe mock for concurrent tests.
type concurrentMockCaller struct {
	response json.RawMessage
	usage    *Usage
	calls    atomic.Int64
}

func (m *concurrentMockCaller) call(_ context.Context, _ *callRequest) (json.RawMessage, *Usage, error) {
	m.calls.Add(1)
	return m.response, m.usage, nil
}

func TestUsage_RecordsOnUnmarshalFailure(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{not valid json}`),
		usage:    &Usage{InputTokens: 100, OutputTokens: 50},
	}
	s := testSession(mock)

	_, err := s.Eval("test").Expect("ok").Judge()
	if err == nil {
		t.Fatal("expected error on bad JSON")
	}

	u := s.Usage()
	if u.Calls != 1 {
		t.Errorf("expected 1 call (tokens spent), got %d", u.Calls)
	}
	if u.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", u.InputTokens)
	}
}
