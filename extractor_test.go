package sense

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// --- ExtractInto behavior tests ---

func TestExtractInto_Basic(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"device": "/dev/sdf",
			"volume_id": "vol-123",
			"message": "already mounted"
		}`),
		usage: &Usage{InputTokens: 200, OutputTokens: 50},
	}
	s := testSession(mock)

	var m mountError
	result, err := s.Extract("device /dev/sdf already mounted with vol-123", &m).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Device != "/dev/sdf" {
		t.Errorf("expected /dev/sdf, got %s", m.Device)
	}
	if m.VolumeID != "vol-123" {
		t.Errorf("expected vol-123, got %s", m.VolumeID)
	}
	if m.Message != "already mounted" {
		t.Errorf("expected 'already mounted', got %s", m.Message)
	}
	if result.TokensUsed != 250 {
		t.Errorf("expected 250 tokens, got %d", result.TokensUsed)
	}
	if result.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", result.Model)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestExtractInto_Nested(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"name": "Alice",
			"age": 30,
			"active": true,
			"address": {"city": "Portland", "state": "OR"}
		}`),
		usage: &Usage{InputTokens: 100, OutputTokens: 50},
	}
	s := testSession(mock)

	var p personRecord
	_, err := s.Extract("Alice, 30, lives in Portland OR", &p).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Alice" {
		t.Errorf("expected Alice, got %s", p.Name)
	}
	if p.Age != 30 {
		t.Errorf("expected 30, got %d", p.Age)
	}
	if !p.Active {
		t.Error("expected active=true")
	}
	if p.Address.City != "Portland" {
		t.Errorf("expected Portland, got %s", p.Address.City)
	}
}

func TestExtractInto_Context(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"device": "/dev/sdf", "volume_id": "vol-123", "message": "mounted"}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	var m mountError
	_, err := s.Extract("some text", &m).Context("AWS EBS errors").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.lastReq == nil {
		t.Fatal("expected a call request")
	}
	if mock.lastReq.userMessage == "" {
		t.Fatal("expected non-empty user message")
	}
}

func TestExtractInto_ModelOverride(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"device": "/dev/sdf", "volume_id": "vol-123", "message": "mounted"}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	var m mountError
	result, err := s.Extract("some text", &m).Model("claude-haiku-4-5-20251001").Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("expected claude-haiku-4-5-20251001, got %s", result.Model)
	}
	if mock.lastReq.model != "claude-haiku-4-5-20251001" {
		t.Errorf("expected model override in request, got %s", mock.lastReq.model)
	}
}

func TestExtractInto_EmptyText(t *testing.T) {
	s := testSession(&mockCaller{})

	var m mountError
	_, err := s.Extract("", &m).Run()
	if !errors.Is(err, ErrNoText) {
		t.Errorf("expected ErrNoText, got %v", err)
	}
}

func TestExtractInto_NilDest(t *testing.T) {
	s := testSession(&mockCaller{})

	_, err := s.Extract("some text", nil).Run()
	if err == nil {
		t.Fatal("expected error for nil dest")
	}
}

func TestExtractInto_NonPointerDest(t *testing.T) {
	s := testSession(&mockCaller{})

	_, err := s.Extract("some text", mountError{}).Run()
	if err == nil {
		t.Fatal("expected error for non-pointer dest")
	}
}

func TestExtractInto_NonStructPointerDest(t *testing.T) {
	s := testSession(&mockCaller{})

	var s2 string
	_, err := s.Extract("some text", &s2).Run()
	if err == nil {
		t.Fatal("expected error for pointer to non-struct")
	}
}

func TestExtractInto_ClientError(t *testing.T) {
	mock := &mockCaller{
		err: ErrRateLimit,
	}
	s := testSession(mock)

	var m mountError
	_, err := s.Extract("some text", &m).Run()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractInto_BadJSON(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`not json`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	var m mountError
	_, err := s.Extract("some text", &m).Run()
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestExtractInto_SkipMode(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	s := testSession(&mockCaller{})

	var m mountError
	m.Device = "should-remain"
	result, err := s.Extract("some text", &m).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result in skip mode")
	}
	// dest is untouched in skip mode
	if m.Device != "should-remain" {
		t.Errorf("expected dest untouched in skip mode, got %s", m.Device)
	}
}

func TestExtractInto_UsageTracking(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"device": "a", "volume_id": "b", "message": "c"}`),
		usage:    &Usage{InputTokens: 300, OutputTokens: 100},
	}
	s := testSession(mock)

	var m mountError
	_, err := s.Extract("text", &m).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	usage := s.Usage()
	if usage.InputTokens != 300 {
		t.Errorf("expected 300 input tokens, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 100 {
		t.Errorf("expected 100 output tokens, got %d", usage.OutputTokens)
	}
	if usage.Calls != 1 {
		t.Errorf("expected 1 call, got %d", usage.Calls)
	}
}

func TestExtractInto_RunContext(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"device": "a", "volume_id": "b", "message": "c"}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	var m mountError
	_, err := s.Extract("text", &m).RunContext(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Device != "a" {
		t.Errorf("expected 'a', got %s", m.Device)
	}
}

func TestExtractInto_ToolName(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"device": "a", "volume_id": "b", "message": "c"}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	var m mountError
	_, err := s.Extract("text", &m).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.lastReq.toolName != "submit_extraction" {
		t.Errorf("expected submit_extraction, got %s", mock.lastReq.toolName)
	}
}

// --- Interface compile-time checks ---

func TestEvaluatorInterface(_ *testing.T) {
	// Compile-time check that Session satisfies Evaluator.
	var _ Evaluator = (*Session)(nil)
}

func TestExtractorInterface(_ *testing.T) {
	// Compile-time check that Session satisfies Extractor.
	var _ Extractor = (*Session)(nil)
}
