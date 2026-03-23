package sense

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// mockCaller returns canned responses for unit tests.
type mockCaller struct {
	response json.RawMessage
	usage    *Usage
	err      error
	calls    int
	lastReq  *callRequest
}

func (m *mockCaller) call(_ context.Context, req *callRequest) (json.RawMessage, *Usage, error) {
	m.calls++
	m.lastReq = req
	return m.response, m.usage, m.err
}

func testSession(m *mockCaller) *Session {
	return &Session{
		client:     m,
		model:      "claude-sonnet-4-6",
		timeout:    0, // no timeout in tests
		maxRetries: 3,
	}
}

// --- Eval with mock ---

func TestEval_AllPass(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"pass": true,
			"score": 1.0,
			"checks": [
				{"expect": "is valid", "pass": true, "confidence": 0.95, "reason": "looks good"}
			]
		}`),
		usage: &Usage{InputTokens: 100, OutputTokens: 50},
	}
	s := testSession(mock)

	result, err := s.Eval("test output").
		Expect("is valid").
		Judge()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Pass {
		t.Error("expected pass")
	}
	if result.Score != 1.0 {
		t.Errorf("expected score 1.0, got %.2f", result.Score)
	}
	if result.TokensUsed != 150 {
		t.Errorf("expected 150 tokens, got %d", result.TokensUsed)
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}
}

func TestEval_MixedResults(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"pass": false,
			"score": 0.5,
			"checks": [
				{"expect": "has intro", "pass": true, "confidence": 0.9, "reason": "found intro"},
				{"expect": "has conclusion", "pass": false, "confidence": 0.85, "reason": "no conclusion found", "evidence": "document ends abruptly"}
			]
		}`),
		usage: &Usage{InputTokens: 200, OutputTokens: 100},
	}
	s := testSession(mock)

	result, err := s.Eval("some doc").
		Expect("has intro").
		Expect("has conclusion").
		Judge()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass {
		t.Error("expected fail")
	}
	if len(result.PassedChecks()) != 1 {
		t.Errorf("expected 1 passed check, got %d", len(result.PassedChecks()))
	}
	if len(result.FailedChecks()) != 1 {
		t.Errorf("expected 1 failed check, got %d", len(result.FailedChecks()))
	}
	if result.FailedChecks()[0].Evidence != "document ends abruptly" {
		t.Errorf("unexpected evidence: %s", result.FailedChecks()[0].Evidence)
	}
}

func TestEval_ClientError(t *testing.T) {
	mock := &mockCaller{
		err: errors.New("connection refused"),
	}
	s := testSession(mock)

	_, err := s.Eval("test").
		Expect("something").
		Judge()
	if err == nil {
		t.Fatal("expected error")
	}

	var senseErr *Error
	if !errors.As(err, &senseErr) {
		t.Fatalf("expected *sense.Error, got %T", err)
	}
	if senseErr.Op != "eval" {
		t.Errorf("expected op=eval, got %s", senseErr.Op)
	}
}

func TestEval_BadJSON(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{not valid json}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	_, err := s.Eval("test").
		Expect("something").
		Judge()
	if err == nil {
		t.Fatal("expected error on bad JSON")
	}

	var senseErr *Error
	if !errors.As(err, &senseErr) {
		t.Fatalf("expected *sense.Error, got %T", err)
	}
}

// --- Compare with mock ---

func TestCompare_AWins(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"winner": "A",
			"score_a": 0.9,
			"score_b": 0.2,
			"criteria": [
				{"name": "quality", "score_a": 0.9, "score_b": 0.2, "winner": "A", "reason": "A is better"}
			],
			"reasoning": "A is clearly superior"
		}`),
		usage: &Usage{InputTokens: 300, OutputTokens: 100},
	}
	s := testSession(mock)

	result, err := s.Compare("good output", "bad output").
		Criteria("quality").
		Judge()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Winner != "A" {
		t.Errorf("expected winner A, got %s", result.Winner)
	}
	if result.TokensUsed != 400 {
		t.Errorf("expected 400 tokens, got %d", result.TokensUsed)
	}
}

func TestCompare_ClientError(t *testing.T) {
	mock := &mockCaller{
		err: errors.New("timeout"),
	}
	s := testSession(mock)

	_, err := s.Compare("a", "b").
		Criteria("quality").
		Judge()
	if err == nil {
		t.Fatal("expected error")
	}

	var senseErr *Error
	if !errors.As(err, &senseErr) {
		t.Fatalf("expected *sense.Error, got %T", err)
	}
	if senseErr.Op != "compare" {
		t.Errorf("expected op=compare, got %s", senseErr.Op)
	}
}

// --- Assert with mock ---

func TestAssert_PassesWithMock(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"pass": true,
			"score": 1.0,
			"checks": [{"expect": "is valid", "pass": true, "confidence": 0.99, "reason": "yes"}]
		}`),
		usage: &Usage{InputTokens: 50, OutputTokens: 50},
	}
	s := testSession(mock)

	s.Assert(t, "test output").
		Expect("is valid").
		Run()
}

func TestRequire_PassesWithMock(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{
			"pass": true,
			"score": 1.0,
			"checks": [{"expect": "is valid", "pass": true, "confidence": 0.99, "reason": "yes"}]
		}`),
		usage: &Usage{InputTokens: 50, OutputTokens: 50},
	}
	s := testSession(mock)

	s.Require(t, "test output").
		Expect("is valid").
		Run()
}

// --- Prompt construction ---

func TestEval_PromptIncludesExpectations(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"pass": true, "score": 1.0, "checks": []}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	_, _ = s.Eval("my output").
		Expect("first thing").
		Expect("second thing").
		Context("background info").
		Judge()

	if mock.lastReq == nil {
		t.Fatal("expected a call to be made")
	}

	msg := mock.lastReq.userMessage
	for _, want := range []string{"my output", "1. first thing", "2. second thing", "background info"} {
		if !contains(msg, want) {
			t.Errorf("prompt missing %q:\n%s", want, msg)
		}
	}
	if mock.lastReq.toolName != "submit_evaluation" {
		t.Errorf("expected tool name submit_evaluation, got %s", mock.lastReq.toolName)
	}
}

func TestCompare_PromptIncludesBothOutputs(t *testing.T) {
	mock := &mockCaller{
		response: json.RawMessage(`{"winner": "tie", "score_a": 0.5, "score_b": 0.5, "criteria": [], "reasoning": "equal"}`),
		usage:    &Usage{},
	}
	s := testSession(mock)

	_, _ = s.Compare("output A here", "output B here").
		Criteria("clarity").
		Context("comparing two drafts").
		Judge()

	if mock.lastReq == nil {
		t.Fatal("expected a call to be made")
	}

	msg := mock.lastReq.userMessage
	for _, want := range []string{"output A here", "output B here", "1. clarity", "comparing two drafts"} {
		if !contains(msg, want) {
			t.Errorf("prompt missing %q:\n%s", want, msg)
		}
	}
	if mock.lastReq.toolName != "submit_comparison" {
		t.Errorf("expected tool name submit_comparison, got %s", mock.lastReq.toolName)
	}
}

// --- Serialization ---

func TestSerializeOutput_String(t *testing.T) {
	got := serializeOutput("hello")
	if got != "hello" {
		t.Errorf("expected hello, got %s", got)
	}
}

func TestSerializeOutput_Bytes(t *testing.T) {
	got := serializeOutput([]byte("bytes"))
	if got != "bytes" {
		t.Errorf("expected bytes, got %s", got)
	}
}

func TestSerializeOutput_Struct(t *testing.T) {
	type S struct {
		Name string `json:"name"`
	}
	got := serializeOutput(S{Name: "test"})
	if !contains(got, `"name": "test"`) {
		t.Errorf("expected JSON with name, got %s", got)
	}
}

func TestSerializeOutput_Stringer(t *testing.T) {
	got := serializeOutput(errors.New("an error"))
	if got != "an error" {
		t.Errorf("expected 'an error', got %s", got)
	}
}

// --- EvalResult formatting ---

func TestEvalResult_String(t *testing.T) {
	r := &EvalResult{
		Pass:  false,
		Score: 0.5,
		Checks: []Check{
			{Expect: "has intro", Pass: true, Confidence: 0.9, Reason: "found it"},
			{Expect: "has conclusion", Pass: false, Confidence: 0.85, Reason: "missing", Evidence: "ends abruptly"},
		},
	}

	s := r.String()
	for _, want := range []string{"1/2 passed", "0.50", "has intro", "has conclusion", "missing", "ends abruptly"} {
		if !contains(s, want) {
			t.Errorf("String() missing %q:\n%s", want, s)
		}
	}
}

func TestCompareResult_String(t *testing.T) {
	r := &CompareResult{
		Winner: "A",
		ScoreA: 0.9,
		ScoreB: 0.2,
		Criteria: []CriterionResult{
			{Name: "quality", ScoreA: 0.9, ScoreB: 0.2, Winner: "A", Reason: "better"},
		},
		Reasoning: "A is clearly better",
	}

	s := r.String()
	for _, want := range []string{"winner=A", "0.90", "0.20", "quality", "better", "clearly better"} {
		if !contains(s, want) {
			t.Errorf("String() missing %q:\n%s", want, s)
		}
	}
}

// --- Error types ---

func TestError_Formatting(t *testing.T) {
	err := &Error{Op: "eval", Message: "api call failed", Err: errors.New("connection refused")}
	got := err.Error()
	if got != "sense eval: api call failed: connection refused" {
		t.Errorf("unexpected: %s", got)
	}

	unwrapped := err.Unwrap()
	if unwrapped == nil || unwrapped.Error() != "connection refused" {
		t.Errorf("unexpected unwrap: %v", unwrapped)
	}
}

func TestError_NoWrapped(t *testing.T) {
	err := &Error{Op: "compare", Message: "no results"}
	got := err.Error()
	if got != "sense compare: no results" {
		t.Errorf("unexpected: %s", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
