//go:build e2e

package sense_test

import (
	"errors"
	"testing"

	"github.com/itsHabib/sense"
)

// --- Assert correctness ---

func TestAssert_PassesOnStructuredReport(t *testing.T) {

	sense.Assert(t, `
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

	sense.Assert(t, `
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

	result, err := sense.Eval("This is a plain paragraph with no structure whatsoever.").
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

	result, err := sense.Eval("The capital of France is Berlin. Python is a compiled language.").
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

	result, err := sense.Eval(`
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

	result, err := sense.Eval("idk lol").
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

	result, err := sense.Eval(`
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

	result, err := sense.Eval("Hello, world!").
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

	result, err := sense.Eval("The sky is blue.").
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

	result, err := sense.Compare(
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

	result, err := sense.Compare(
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

	result, err := sense.Compare(
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

	result, err := sense.Eval(resp).
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
	_, err := sense.Eval("anything").Judge()
	if err == nil {
		t.Fatal("expected error when no expectations provided")
	}
	if !errors.Is(err, sense.ErrNoExpectations) {
		t.Errorf("expected ErrNoExpectations, got: %v", err)
	}
}

func TestCompare_NoCriteriaReturnsError(t *testing.T) {
	_, err := sense.Compare("a", "b").Judge()
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
	sense.Assert(t, "anything").
		Expect("impossible expectation").
		Run()
}

func TestSkipMode_Require(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")
	sense.Require(t, "anything").
		Expect("impossible expectation").
		Run()
}

func TestSkipMode_EvalReturnsPass(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	result, err := sense.Eval("anything").
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

	result, err := sense.Compare("a", "b").
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

	sense.Require(t, "Hello, world!").
		Expect("contains a greeting").
		Run()
}

// --- Metadata tracking ---

func TestEval_TracksMetadata(t *testing.T) {

	result, err := sense.Eval("test input").
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
