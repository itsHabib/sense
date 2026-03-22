package sense

import (
	"context"
	"testing"
)

// AssertBuilder constructs and executes a test assertion.
type AssertBuilder struct {
	t     testing.TB
	eval  *EvalBuilder
	fatal bool
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
			b.t.Fatalf("sense assertion error: %v", err)
		} else {
			b.t.Errorf("sense assertion error: %v", err)
		}
		return
	}

	if !result.Pass {
		if b.fatal {
			b.t.Fatal(result.FormatResult())
		} else {
			b.t.Error(result.FormatResult())
		}
	}
}
