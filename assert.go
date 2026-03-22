package sense

import (
	"context"
	"testing"
)

// AssertBuilder constructs and executes a test assertion.
// Calls t.Fatal() if any expectation fails.
type AssertBuilder struct {
	t    testing.TB
	eval *EvalBuilder
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

// Run executes the assertion. Calls t.Fatal() on failure.
func (b *AssertBuilder) Run() {
	b.RunContext(context.Background())
}

// RunContext executes the assertion with the given context.
func (b *AssertBuilder) RunContext(ctx context.Context) {
	b.t.Helper()

	result, err := b.eval.JudgeContext(ctx)
	if err != nil {
		b.t.Fatalf("agent assertion error: %v", err)
		return
	}

	if !result.Pass {
		b.t.Fatal(result.FormatResult())
	}
}
