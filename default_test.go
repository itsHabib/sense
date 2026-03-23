package sense_test

import (
	"errors"
	"testing"

	"github.com/itsHabib/sense"
)

func TestPackageAssert_SkipMode(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	sense.Assert(t, "anything").
		Expect("impossible expectation").
		Run()
}

func TestPackageRequire_SkipMode(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	sense.Require(t, "anything").
		Expect("impossible expectation").
		Run()
}

func TestPackageEval_SkipMode(t *testing.T) {
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

func TestPackageCompare_NoCriteria(t *testing.T) {
	_, err := sense.Compare("a", "b").Judge()
	if !errors.Is(err, sense.ErrNoCriteria) {
		t.Errorf("expected ErrNoCriteria, got: %v", err)
	}
}

func TestPackageEval_NoExpectations(t *testing.T) {
	_, err := sense.Eval("hello").Judge()
	if !errors.Is(err, sense.ErrNoExpectations) {
		t.Errorf("expected ErrNoExpectations, got: %v", err)
	}
}
