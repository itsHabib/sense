package sense_test

import (
	"os"
	"testing"

	agent "github.com/itsHabib/sense"
)

func TestAssertSmoke(t *testing.T) {
	if os.Getenv("SENSE_INTEGRATION") == "" {
		t.Skip("set SENSE_INTEGRATION=1 to run integration tests")
	}

	agent.Assert(t, "Hello, world! Welcome to our platform.").
		Expect("contains a greeting").
		Expect("is not empty").
		Run()
}

func TestEvalSmoke(t *testing.T) {
	if os.Getenv("SENSE_INTEGRATION") == "" {
		t.Skip("set SENSE_INTEGRATION=1 to run integration tests")
	}

	result, err := agent.Eval("The quick brown fox jumps over the lazy dog.").
		Expect("is a complete sentence").
		Expect("mentions an animal").
		Expect("contains a number").
		Judge()
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	// Should pass first two, fail the third
	if result.Pass {
		t.Log("all checks passed (unexpected — 'contains a number' should fail)")
	}

	t.Logf("score: %.2f, checks: %d", result.Score, len(result.Checks))
	for _, c := range result.Checks {
		status := "PASS"
		if !c.Pass {
			status = "FAIL"
		}
		t.Logf("  [%s] %s (confidence: %.2f) — %s", status, c.Expect, c.Confidence, c.Reason)
	}
}

func TestCompareSmoke(t *testing.T) {
	if os.Getenv("SENSE_INTEGRATION") == "" {
		t.Skip("set SENSE_INTEGRATION=1 to run integration tests")
	}

	result, err := agent.Compare(
		"Go is a statically typed, compiled language designed at Google.",
		"go is good",
	).
		Criteria("completeness").
		Criteria("professionalism").
		Judge()
	if err != nil {
		t.Fatalf("compare failed: %v", err)
	}

	t.Logf("winner: %s (A: %.2f, B: %.2f)", result.Winner, result.ScoreA, result.ScoreB)
	t.Logf("reasoning: %s", result.Reasoning)
}

func TestSkipMode(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	// Should not make any API calls
	agent.Assert(t, "anything").
		Expect("this would normally need an API call").
		Run()
}

func TestEvalNoExpectations(t *testing.T) {
	_, err := agent.Eval("hello").Judge()
	if err == nil {
		t.Fatal("expected error for no expectations")
	}
}

func TestEvalSkipMode(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	result, err := agent.Eval("anything").
		Expect("something").
		Judge()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Pass {
		t.Fatal("skip mode should always pass")
	}
}
