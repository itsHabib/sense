package sense_test

import (
	"errors"
	"testing"

	"github.com/itsHabib/sense"
)

func TestEvalNoExpectations(t *testing.T) {
	_, err := sense.Eval("hello").Judge()
	if err == nil {
		t.Fatal("expected error for no expectations")
	}
	if !errors.Is(err, sense.ErrNoExpectations) {
		t.Errorf("expected ErrNoExpectations, got: %v", err)
	}
}

func TestCompareNoCriteria(t *testing.T) {
	_, err := sense.Compare("a", "b").Judge()
	if err == nil {
		t.Fatal("expected error for no criteria")
	}
	if !errors.Is(err, sense.ErrNoCriteria) {
		t.Errorf("expected ErrNoCriteria, got: %v", err)
	}
}

func TestSkipModeAssert(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")
	sense.Assert(t, "anything").
		Expect("impossible expectation").
		Run()
}

func TestSkipModeEval(t *testing.T) {
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
}
