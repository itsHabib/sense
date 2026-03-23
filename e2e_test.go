//go:build e2e

package sense_test

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/itsHabib/sense"
)

var s *sense.Session

func TestMain(m *testing.M) {
	s = sense.NewSession(sense.Config{})
	code := m.Run()
	t := s.Usage()
	if t.Calls > 0 {
		// Print session cost summary after all tests complete.
		println(t.String())
	}
	s.Close()
	os.Exit(code)
}

// --- Assert correctness ---

func TestAssert_PassesOnStructuredReport(t *testing.T) {
	t.Parallel()

	s.Assert(t,`
# Quarterly Report

## Summary
Revenue grew 15% year-over-year to $4.2M. Customer churn decreased from 8% to 5%.

## Key Metrics
- ARR: $4.2M
- Churn: 5%
- NPS: 72

## Recommendations
1. Invest in customer success to maintain low churn
2. Expand into APAC market in Q3
3. Hire 3 senior engineers for the platform team
	`).
		Expect("contains a summary section").
		Expect("includes specific financial metrics").
		Expect("has actionable recommendations").
		Expect("is structured with headings").
		Context("this is a quarterly business report").
		Run()
}

func TestAssert_PassesOnValidGoCode(t *testing.T) {
	t.Parallel()

	s.Assert(t,`
package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("server failed: %v\n", err)
	}
}
	`).
		Expect("is valid Go code").
		Expect("has a main function").
		Expect("includes error handling").
		Expect("uses the net/http package").
		Context("task was to write a simple HTTP server").
		Run()
}

// --- Eval should-fail ---

func TestEval_FailsOnPlainText(t *testing.T) {
	t.Parallel()

	result, err := s.Eval("This is a plain paragraph with no structure whatsoever.").
		Expect("contains bullet points").
		Expect("has at least 3 sections with headings").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	if result.Pass {
		t.Fatal("expected evaluation to fail — plain text has no bullet points or headings")
	}
	if result.Score > 0.3 {
		t.Errorf("expected score < 0.3, got %.2f", result.Score)
	}
}

func TestEval_FailsOnFactualErrors(t *testing.T) {
	t.Parallel()

	result, err := s.Eval("The capital of France is Berlin. Python is a compiled language.").
		Expect("all factual claims are correct").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	if result.Pass {
		t.Fatal("expected evaluation to fail — content contains factual errors")
	}

	failed := result.FailedChecks()
	if len(failed) == 0 {
		t.Fatal("expected at least one failed check")
	}
	if failed[0].Confidence < 0.8 {
		t.Errorf("expected high confidence on factual error, got %.2f", failed[0].Confidence)
	}
}

// --- Eval score accuracy ---

func TestEval_HighScoreForGoodOutput(t *testing.T) {
	t.Parallel()

	result, err := s.Eval(`
func Add(a, b int) int {
	return a + b
}

func TestAdd(t *testing.T) {
	tests := []struct{
		a, b, want int
	}{
		{1, 2, 3},
		{0, 0, 0},
		{-1, 1, 0},
	}
	for _, tt := range tests {
		if got := Add(tt.a, tt.b); got != tt.want {
			t.Errorf("Add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
	`).
		Expect("includes a function implementation").
		Expect("has table-driven tests").
		Expect("tests edge cases like zero and negative numbers").
		Context("task was to write a Go function with tests").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	if !result.Pass {
		t.Errorf("expected all checks to pass:\n%s", result.String())
	}
	if result.Score < 0.9 {
		t.Errorf("expected score > 0.9, got %.2f", result.Score)
	}
}

func TestEval_LowScoreForBadOutput(t *testing.T) {
	t.Parallel()

	result, err := s.Eval("idk lol").
		Expect("is a well-structured technical document").
		Expect("includes diagrams or code examples").
		Expect("has a table of contents").
		Context("task was to write an architecture document").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	if result.Pass {
		t.Fatal("expected all checks to fail for garbage input")
	}
	if result.Score > 0.1 {
		t.Errorf("expected score < 0.1, got %.2f", result.Score)
	}
}

func TestEval_MixedPassFail(t *testing.T) {
	t.Parallel()

	result, err := s.Eval(`
# API Design

The API uses REST over HTTP with JSON payloads.

## Endpoints

GET /users - list users
POST /users - create user
	`).
		Expect("describes an API").
		Expect("mentions REST or HTTP").
		Expect("includes authentication details").
		Expect("has rate limiting documentation").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	if result.Pass {
		t.Error("expected mixed results — auth and rate limiting are missing")
	}

	passed := result.PassedChecks()
	failed := result.FailedChecks()

	if len(passed) < 2 {
		t.Errorf("expected at least 2 passing checks, got %d", len(passed))
	}
	if len(failed) < 2 {
		t.Errorf("expected at least 2 failing checks, got %d", len(failed))
	}
}

// --- Eval confidence ---

func TestEval_HighConfidenceOnObviousPass(t *testing.T) {
	t.Parallel()

	result, err := s.Eval("Hello, world!").
		Expect("contains the word hello").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	if !result.Pass {
		t.Fatal("expected obvious check to pass")
	}
	if result.Checks[0].Confidence < 0.9 {
		t.Errorf("expected confidence > 0.9, got %.2f", result.Checks[0].Confidence)
	}
}

func TestEval_HighConfidenceOnObviousFail(t *testing.T) {
	t.Parallel()

	result, err := s.Eval("The sky is blue.").
		Expect("discusses quantum mechanics").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	if result.Pass {
		t.Fatal("expected obvious check to fail")
	}
	if result.Checks[0].Confidence < 0.9 {
		t.Errorf("expected confidence > 0.9, got %.2f", result.Checks[0].Confidence)
	}
}

// --- Compare ---

func TestCompare_PicksObviousWinner(t *testing.T) {
	t.Parallel()

	result, err := s.Compare(
		`Go is a statically typed, compiled programming language designed at Google
by Robert Griesemer, Rob Pike, and Ken Thompson. It features garbage collection,
structural typing, and CSP-style concurrency with goroutines and channels.`,
		"go is a language",
	).
		Criteria("completeness").
		Criteria("informativeness").
		Judge()
	if err != nil {
		t.Fatalf("compare error: %v", err)
	}

	if result.Winner != "A" {
		t.Errorf("expected A to win, got %s", result.Winner)
	}
	if result.ScoreA < 0.7 {
		t.Errorf("expected A score > 0.7, got %.2f", result.ScoreA)
	}
	if result.ScoreB > 0.3 {
		t.Errorf("expected B score < 0.3, got %.2f", result.ScoreB)
	}
}

func TestCompare_CloseScoresOnSimilarOutputs(t *testing.T) {
	t.Parallel()

	result, err := s.Compare(
		"Go is a compiled language with garbage collection.",
		"Go is a garbage-collected, compiled programming language.",
	).
		Criteria("accuracy").
		Criteria("completeness").
		Judge()
	if err != nil {
		t.Fatalf("compare error: %v", err)
	}

	diff := result.ScoreA - result.ScoreB
	if diff > 0.3 || diff < -0.3 {
		t.Errorf("expected close scores for similar outputs, got A=%.2f B=%.2f", result.ScoreA, result.ScoreB)
	}
}

func TestCompare_SplitCriteria(t *testing.T) {
	t.Parallel()

	result, err := s.Compare(
		"The HTTP 404 status code indicates that the server cannot find the requested resource. The URI is valid but the resource itself does not exist.",
		"Oops! Looks like we couldn't find what you're looking for. The page might have been moved or deleted. Try going back to the homepage!",
	).
		Criteria("technical accuracy").
		Criteria("user friendliness").
		Judge()
	if err != nil {
		t.Fatalf("compare error: %v", err)
	}

	if len(result.Criteria) < 2 {
		t.Fatalf("expected at least 2 criteria results, got %d", len(result.Criteria))
	}

	for _, c := range result.Criteria {
		t.Logf("  %s: A=%.2f B=%.2f winner=%s", c.Name, c.ScoreA, c.ScoreB, c.Winner)
	}
}

// --- Struct input ---

func TestEval_StructInput(t *testing.T) {
	t.Parallel()

	type APIResponse struct {
		Status  string `json:"status"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	resp := APIResponse{
		Status:  "error",
		Code:    429,
		Message: "rate limit exceeded, retry after 30 seconds",
	}

	result, err := s.Eval(resp).
		Expect("indicates a rate limiting error").
		Expect("includes retry information").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	if !result.Pass {
		t.Errorf("expected struct input to be evaluated correctly:\n%s", result.String())
	}
}

// --- Error handling (no API) ---

func TestEval_NoExpectationsReturnsError(t *testing.T) {
	_, err := s.Eval("anything").Judge()
	if err == nil {
		t.Fatal("expected error when no expectations provided")
	}
	if !errors.Is(err, sense.ErrNoExpectations) {
		t.Errorf("expected ErrNoExpectations, got: %v", err)
	}
}

func TestCompare_NoCriteriaReturnsError(t *testing.T) {
	_, err := s.Compare("a", "b").Judge()
	if err == nil {
		t.Fatal("expected error when no criteria provided")
	}
	if !errors.Is(err, sense.ErrNoCriteria) {
		t.Errorf("expected ErrNoCriteria, got: %v", err)
	}
}

// --- Skip mode (no API) ---

func TestSkipMode_Assert(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")
	s.Assert(t,"anything").
		Expect("impossible expectation").
		Run()
}

func TestSkipMode_Require(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")
	s.Require(t,"anything").
		Expect("impossible expectation").
		Run()
}

func TestSkipMode_EvalReturnsPass(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	result, err := s.Eval("anything").
		Expect("something").
		Judge()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Pass {
		t.Fatal("skip mode should always pass")
	}
	if result.Score != 1.0 {
		t.Errorf("skip mode score should be 1.0, got %.2f", result.Score)
	}
}

func TestSkipMode_CompareReturnsTie(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	result, err := s.Compare("a", "b").
		Criteria("anything").
		Judge()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Winner != "tie" {
		t.Errorf("skip mode should return tie, got %s", result.Winner)
	}
}

// --- Require ---

func TestRequire_PassesOnValidInput(t *testing.T) {
	t.Parallel()

	s.Require(t,"Hello, world!").
		Expect("contains a greeting").
		Run()
}

// --- Metadata tracking ---

func TestEval_TracksMetadata(t *testing.T) {
	t.Parallel()

	result, err := s.Eval("test input").
		Expect("is not empty").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	if result.TokensUsed == 0 {
		t.Error("expected non-zero token usage")
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
	if result.Model == "" {
		t.Error("expected model to be set")
	}
	t.Logf("tokens: %d, duration: %s, model: %s", result.TokensUsed, result.Duration, result.Model)
}

// --- Usage tracking ---

func TestUsage_TracksRealAPICalls(t *testing.T) {
	t.Parallel()

	// Snapshot usage before and after a real API call.
	before := s.Usage()

	_, err := s.Eval("The sky is blue.").
		Expect("makes a factual claim").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	after := s.Usage()
	if after.Calls <= before.Calls {
		t.Errorf("expected calls to increase: before=%d after=%d", before.Calls, after.Calls)
	}
	if after.InputTokens <= before.InputTokens {
		t.Errorf("expected input tokens to increase: before=%d after=%d", before.InputTokens, after.InputTokens)
	}
	if after.OutputTokens <= before.OutputTokens {
		t.Errorf("expected output tokens to increase: before=%d after=%d", before.OutputTokens, after.OutputTokens)
	}
	if after.TotalTokens != after.InputTokens+after.OutputTokens {
		t.Errorf("TotalTokens (%d) != InputTokens (%d) + OutputTokens (%d)",
			after.TotalTokens, after.InputTokens, after.OutputTokens)
	}
	t.Logf("session usage after this call: %s", after.String())
}

// --- Extract ---

func TestExtract_AWSError(t *testing.T) {
	t.Parallel()

	type mountErr struct {
		Device   string `json:"device" sense:"The device path e.g. /dev/sdf"`
		VolumeID string `json:"volume_id" sense:"The EBS volume ID e.g. vol-abc123"`
		Message  string `json:"message" sense:"The error message"`
	}

	result, err := sense.Extract[mountErr](s,
		"attach volume: device /dev/sdf is already in use by vol-0abc123def456").
		Context("AWS EC2 EBS error messages").
		Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if result.Data.Device == "" {
		t.Error("expected device to be extracted")
	}
	if result.Data.VolumeID == "" {
		t.Error("expected volume ID to be extracted")
	}
	if result.TokensUsed == 0 {
		t.Error("expected non-zero token usage")
	}
	t.Logf("extracted: device=%s volume=%s message=%q (tokens: %d)",
		result.Data.Device, result.Data.VolumeID, result.Data.Message, result.TokensUsed)
}

func TestExtract_LogLine(t *testing.T) {
	t.Parallel()

	type source struct {
		File string `json:"file"`
		Line int    `json:"line"`
	}
	type logEntry struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		Source  source `json:"source"`
	}

	result, err := sense.Extract[logEntry](s,
		`2024-03-15 14:22:01 ERROR [server.go:142] connection pool exhausted: max 50 connections reached`).
		Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if result.Data.Level == "" {
		t.Error("expected level to be extracted")
	}
	if result.Data.Source.File == "" {
		t.Error("expected source file to be extracted")
	}
	if result.Data.Source.Line == 0 {
		t.Error("expected source line to be extracted")
	}
	t.Logf("extracted: level=%s message=%q source=%s:%d",
		result.Data.Level, result.Data.Message, result.Data.Source.File, result.Data.Source.Line)
}

func TestExtract_OptionalFieldsMissing(t *testing.T) {
	t.Parallel()

	type contactInfo struct {
		Name  string  `json:"name"`
		Email *string `json:"email" sense:"Email address if present"`
		Phone *string `json:"phone" sense:"Phone number if present"`
	}

	result, err := sense.Extract[contactInfo](s,
		"Please contact Alice regarding the account issue").
		Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if result.Data.Name == "" {
		t.Error("expected name to be extracted")
	}
	// Email and phone should be nil — not present in text
	t.Logf("extracted: name=%s email=%v phone=%v",
		result.Data.Name, result.Data.Email, result.Data.Phone)
}

func TestExtract_KubernetesEvent(t *testing.T) {
	t.Parallel()

	type k8sEvent struct {
		Kind      string   `json:"kind" sense:"Resource kind (Pod, Deployment, Service, etc)"`
		Name      string   `json:"name" sense:"Resource name"`
		Namespace string   `json:"namespace"`
		Reason    string   `json:"reason" sense:"Event reason (CrashLoopBackOff, OOMKilled, etc)"`
		Message   string   `json:"message"`
		Restarts  *int     `json:"restarts" sense:"Container restart count if mentioned"`
		ExitCode  *int     `json:"exit_code" sense:"Container exit code if mentioned"`
		Images    []string `json:"images" sense:"Container image names if mentioned"`
	}

	result, err := sense.Extract[k8sEvent](s,
		`Warning  BackOff  pod/api-server-7b4d5f6-x2k9p  namespace=production  Back-off restarting failed container api-server in pod api-server-7b4d5f6-x2k9p_production(abc123): container "api-server" (image: registry.io/api:v2.3.1) exited with code 137, restart count 14`).
		Context("Kubernetes event log output from kubectl get events").
		Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if result.Data.Kind == "" {
		t.Error("expected kind to be extracted")
	}
	if result.Data.Name == "" {
		t.Error("expected name to be extracted")
	}
	if result.Data.Namespace == "" {
		t.Error("expected namespace to be extracted")
	}
	if result.Data.Restarts == nil {
		t.Error("expected restart count to be extracted")
	} else if *result.Data.Restarts != 14 {
		t.Errorf("expected 14 restarts, got %d", *result.Data.Restarts)
	}
	if result.Data.ExitCode == nil {
		t.Error("expected exit code to be extracted")
	} else if *result.Data.ExitCode != 137 {
		t.Errorf("expected exit code 137, got %d", *result.Data.ExitCode)
	}
	t.Logf("extracted: kind=%s name=%s ns=%s reason=%s restarts=%v exit=%v images=%v",
		result.Data.Kind, result.Data.Name, result.Data.Namespace,
		result.Data.Reason, result.Data.Restarts, result.Data.ExitCode, result.Data.Images)
}

func TestExtract_SQLError(t *testing.T) {
	t.Parallel()

	type sqlErr struct {
		ErrorCode  int      `json:"error_code" sense:"Database-specific error code"`
		Table      string   `json:"table" sense:"Table name involved"`
		Constraint *string  `json:"constraint" sense:"Constraint name if a constraint violation"`
		Columns    []string `json:"columns" sense:"Column names involved"`
		Message    string   `json:"message"`
	}

	result, err := sense.Extract[sqlErr](s,
		`pq: duplicate key value violates unique constraint "users_email_key" on table "users" (SQLSTATE 23505): Key (email)=(alice@example.com) already exists.`).
		Context("PostgreSQL error messages").
		Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if result.Data.Table == "" {
		t.Error("expected table to be extracted")
	}
	if result.Data.Constraint == nil || *result.Data.Constraint == "" {
		t.Error("expected constraint name to be extracted")
	}
	if len(result.Data.Columns) == 0 {
		t.Error("expected at least one column")
	}
	t.Logf("extracted: code=%d table=%s constraint=%v columns=%v message=%q",
		result.Data.ErrorCode, result.Data.Table, result.Data.Constraint,
		result.Data.Columns, result.Data.Message)
}

func TestExtract_GitCommit(t *testing.T) {
	t.Parallel()

	type fileChange struct {
		Path      string `json:"path"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
	}
	type gitCommit struct {
		Hash    string       `json:"hash" sense:"Short or full commit hash"`
		Author  string       `json:"author"`
		Date    string       `json:"date" sense:"Commit date in any format"`
		Message string       `json:"message" sense:"Commit message (first line only)"`
		Files   []fileChange `json:"files" sense:"Files changed with line counts"`
	}

	result, err := sense.Extract[gitCommit](s, `
commit a1b2c3d (HEAD -> main)
Author: Alice Smith <alice@example.com>
Date:   Mon Mar 15 14:22:01 2024 -0700

    fix: resolve race condition in connection pool

 src/pool.go    | 42 ++++++++++++++++++++++++------------------
 src/pool_test.go | 18 ++++++++++++++++++
 2 files changed, 42 insertions(+), 18 deletions(-)`).
		Context("git log --stat output").
		Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if result.Data.Hash == "" {
		t.Error("expected hash")
	}
	if result.Data.Author == "" {
		t.Error("expected author")
	}
	if result.Data.Message == "" {
		t.Error("expected message")
	}
	if len(result.Data.Files) == 0 {
		t.Error("expected file changes")
	}
	t.Logf("extracted: hash=%s author=%s message=%q files=%d",
		result.Data.Hash, result.Data.Author, result.Data.Message, len(result.Data.Files))
	for _, f := range result.Data.Files {
		t.Logf("  %s +%d -%d", f.Path, f.Additions, f.Deletions)
	}
}

func TestExtract_NginxAccessLog(t *testing.T) {
	t.Parallel()

	type accessLog struct {
		IP        string  `json:"ip" sense:"Client IP address"`
		Method    string  `json:"method" sense:"HTTP method"`
		Path      string  `json:"path" sense:"Request path"`
		Status    int     `json:"status" sense:"HTTP response status code"`
		BodyBytes int     `json:"body_bytes" sense:"Response body size in bytes"`
		Referrer  *string `json:"referrer" sense:"HTTP referrer if present"`
		UserAgent string  `json:"user_agent"`
		Latency   *string `json:"latency" sense:"Request duration if present"`
	}

	result, err := sense.Extract[accessLog](s,
		`192.168.1.42 - - [15/Mar/2024:14:22:01 +0000] "POST /api/v2/users HTTP/1.1" 201 1842 "https://app.example.com/dashboard" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)" rt=0.042`).
		Context("Nginx combined access log format with request time").
		Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if result.Data.IP == "" {
		t.Error("expected IP")
	}
	if result.Data.Method == "" {
		t.Error("expected method")
	}
	if result.Data.Path == "" {
		t.Error("expected path")
	}
	if result.Data.Status != 201 {
		t.Errorf("expected status 201, got %d", result.Data.Status)
	}
	if result.Data.BodyBytes == 0 {
		t.Error("expected non-zero body bytes")
	}
	t.Logf("extracted: %s %s %s → %d (%d bytes) referrer=%v ua=%s latency=%v",
		result.Data.IP, result.Data.Method, result.Data.Path,
		result.Data.Status, result.Data.BodyBytes, result.Data.Referrer,
		result.Data.UserAgent, result.Data.Latency)
}

// --- ExtractInto (method-style) ---

func TestExtractInto_AWSError(t *testing.T) {
	t.Parallel()

	type mountErr struct {
		Device   string `json:"device" sense:"The device path e.g. /dev/sdf"`
		VolumeID string `json:"volume_id" sense:"The EBS volume ID e.g. vol-abc123"`
		Message  string `json:"message" sense:"The error message"`
	}

	var m mountErr
	result, err := s.Extract(
		"attach volume: device /dev/sdf is already in use by vol-0abc123def456", &m).
		Context("AWS EC2 EBS error messages").
		Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if m.Device == "" {
		t.Error("expected device to be extracted")
	}
	if m.VolumeID == "" {
		t.Error("expected volume ID to be extracted")
	}
	if result.TokensUsed == 0 {
		t.Error("expected non-zero token usage")
	}
	t.Logf("extracted: device=%s volume=%s message=%q (tokens: %d)",
		m.Device, m.VolumeID, m.Message, result.TokensUsed)
}

func TestExtractInto_LogLine(t *testing.T) {
	t.Parallel()

	type source struct {
		File string `json:"file"`
		Line int    `json:"line"`
	}
	type logEntry struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		Source  source `json:"source"`
	}

	var entry logEntry
	_, err := s.Extract(
		`2024-03-15 14:22:01 ERROR [server.go:142] connection pool exhausted: max 50 connections reached`,
		&entry).Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if entry.Level == "" {
		t.Error("expected level to be extracted")
	}
	if entry.Source.File == "" {
		t.Error("expected source file to be extracted")
	}
	if entry.Source.Line == 0 {
		t.Error("expected source line to be extracted")
	}
	t.Logf("extracted: level=%s message=%q source=%s:%d",
		entry.Level, entry.Message, entry.Source.File, entry.Source.Line)
}

func TestExtractInto_KubernetesEvent(t *testing.T) {
	t.Parallel()

	type k8sEvent struct {
		Kind      string   `json:"kind" sense:"Resource kind (Pod, Deployment, Service, etc)"`
		Name      string   `json:"name" sense:"Resource name"`
		Namespace string   `json:"namespace"`
		Reason    string   `json:"reason" sense:"Event reason (CrashLoopBackOff, OOMKilled, etc)"`
		Message   string   `json:"message"`
		Restarts  *int     `json:"restarts" sense:"Container restart count if mentioned"`
		ExitCode  *int     `json:"exit_code" sense:"Container exit code if mentioned"`
		Images    []string `json:"images" sense:"Container image names if mentioned"`
	}

	var event k8sEvent
	_, err := s.Extract(
		`Warning  BackOff  pod/api-server-7b4d5f6-x2k9p  namespace=production  Back-off restarting failed container api-server in pod api-server-7b4d5f6-x2k9p_production(abc123): container "api-server" (image: registry.io/api:v2.3.1) exited with code 137, restart count 14`,
		&event).
		Context("Kubernetes event log output from kubectl get events").
		Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if event.Kind == "" {
		t.Error("expected kind to be extracted")
	}
	if event.Name == "" {
		t.Error("expected name to be extracted")
	}
	if event.Namespace == "" {
		t.Error("expected namespace to be extracted")
	}
	if event.Restarts == nil {
		t.Error("expected restart count to be extracted")
	} else if *event.Restarts != 14 {
		t.Errorf("expected 14 restarts, got %d", *event.Restarts)
	}
	if event.ExitCode == nil {
		t.Error("expected exit code to be extracted")
	} else if *event.ExitCode != 137 {
		t.Errorf("expected exit code 137, got %d", *event.ExitCode)
	}
	t.Logf("extracted: kind=%s name=%s ns=%s reason=%s restarts=%v exit=%v images=%v",
		event.Kind, event.Name, event.Namespace,
		event.Reason, event.Restarts, event.ExitCode, event.Images)
}

func TestExtractInto_SkipMode(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	type simple struct {
		Name string `json:"name"`
	}

	var v simple
	v.Name = "untouched"
	result, err := s.Extract("anything", &v).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result in skip mode")
	}
	if v.Name != "untouched" {
		t.Errorf("expected dest untouched in skip mode, got %s", v.Name)
	}
}

func TestExtractInto_ValidationErrors(t *testing.T) {
	_, err := s.Extract("text", nil).Run()
	if err == nil {
		t.Error("expected error for nil dest")
	}

	_, err = s.Extract("text", "not a pointer").Run()
	if err == nil {
		t.Error("expected error for non-pointer dest")
	}

	var str string
	_, err = s.Extract("text", &str).Run()
	if err == nil {
		t.Error("expected error for pointer to non-struct")
	}
}

// --- Evaluator / Extractor interfaces ---

func TestEvaluatorInterface(t *testing.T) {
	// Verify Session satisfies Evaluator at runtime.
	var e sense.Evaluator = s
	result, err := e.Eval("Hello, world!").
		Expect("contains a greeting").
		Judge()
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	if !result.Pass {
		t.Error("expected pass via Evaluator interface")
	}
}

func TestExtractorInterface(t *testing.T) {
	// Verify Session satisfies Extractor at runtime.
	var e sense.Extractor = s

	type greeting struct {
		Word string `json:"word" sense:"The greeting word"`
	}

	var g greeting
	_, err := e.Extract("Hello, world!", &g).Run()
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}
	if g.Word == "" {
		t.Error("expected word to be extracted via Extractor interface")
	}
	t.Logf("extracted via interface: word=%s", g.Word)
}

// Verify both interfaces can be composed.
func TestBothInterfaces(_ *testing.T) {
	type both interface {
		sense.Evaluator
		sense.Extractor
	}
	var b both = s
	_ = fmt.Sprintf("%T", b) // use b to avoid unused var
}
