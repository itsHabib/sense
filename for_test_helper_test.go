package sense_test

import (
	"testing"

	"github.com/itsHabib/sense"
)

func TestForTest_SkipMode(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	s := sense.ForTest(t)
	s.Assert(t, "anything").
		Expect("impossible expectation").
		Run()
}

func TestForTest_WithModel(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	s := sense.ForTest(t, sense.WithModel("claude-haiku-4-5-20251001"))
	s.Assert(t, "anything").
		Expect("works").
		Run()
}

func TestForTest_UsageTracking(t *testing.T) {
	t.Setenv("SENSE_SKIP", "1")

	s := sense.ForTest(t)

	// skip mode doesn't make API calls, so usage should be zero
	u := s.Usage()
	if u.Calls != 0 {
		t.Errorf("expected 0 calls, got %d", u.Calls)
	}
}
