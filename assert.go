package sense

import (
	"context"
	"testing"
	"time"
)

// AssertBuilder constructs and executes a test assertion.
type AssertBuilder struct {
	t     testing.TB
	eval  *EvalBuilder
	fatal bool
	usage *Usage
}

// Expect adds a natural language expectation. Chainable.
func (b *AssertBuilder) Expect(expectation string) *AssertBuilder {
	b.eval.Expect(expectation)
	return b
}

// Context adds background information for the judge.
func (b *AssertBuilder) Context(ctx string) *AssertBuilder {
	b.eval.Context(ctx)
	return b
}

// Model overrides the judge model for this assertion.
func (b *AssertBuilder) Model(model string) *AssertBuilder {
	b.eval.Model(model)
	return b
}

// Timeout overrides the per-call timeout for this assertion.
func (b *AssertBuilder) Timeout(d time.Duration) *AssertBuilder {
	b.eval.Timeout(d)
	return b
}

// MinConfidence sets the minimum confidence threshold for this assertion.
// Checks that pass Claude's judgment but fall below this threshold are
// treated as failures.
func (b *AssertBuilder) MinConfidence(threshold float64) *AssertBuilder {
	b.eval.MinConfidence(threshold)
	return b
}

// Usage captures the token usage from the API call into the provided pointer.
// This is useful for Assert/Require which don't return a result.
//
//	var u sense.Usage
//	sense.Assert(t, output).Expect("...").Usage(&u).Run()
//	fmt.Println(u.InputTokens, u.OutputTokens)
func (b *AssertBuilder) Usage(u *Usage) *AssertBuilder {
	b.usage = u
	return b
}

// Run executes the assertion.
func (b *AssertBuilder) Run() {
	b.RunContext(context.Background())
}

// RunContext executes the assertion with the given context.
func (b *AssertBuilder) RunContext(ctx context.Context) {
	b.t.Helper()

	result, err := b.eval.JudgeContext(ctx)
	if err != nil {
		if b.fatal {
			b.t.Fatalf("%v", err)
		} else {
			b.t.Errorf("%v", err)
		}
		return
	}

	if b.usage != nil {
		*b.usage = result.Usage
	}

	if !result.Pass {
		if b.fatal {
			b.t.Fatal(result.String())
		} else {
			b.t.Error(result.String())
		}
	}
}
