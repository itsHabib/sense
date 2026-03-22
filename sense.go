// Package sense provides agent-powered test assertions for Go.
//
// Sense lets you write natural language assertions that an AI judge evaluates.
// Deterministic pass/fail from non-deterministic output.
//
//	sense.Assert(t, doc).
//	    Expect("covers all sections from the brief").
//	    Expect("includes actionable recommendations").
//	    Run()
package sense

import "testing"

// Assert creates a test assertion that calls t.Error() on failure (test continues).
// The output can be a string, []byte, or any type (structs are serialized to JSON).
func Assert(t testing.TB, output any) *AssertBuilder {
	return &AssertBuilder{
		t:     t,
		eval:  &EvalBuilder{output: output},
		fatal: false,
	}
}

// Require creates a test assertion that calls t.Fatal() on failure (test stops).
// The output can be a string, []byte, or any type (structs are serialized to JSON).
func Require(t testing.TB, output any) *AssertBuilder {
	return &AssertBuilder{
		t:     t,
		eval:  &EvalBuilder{output: output},
		fatal: true,
	}
}

// Eval creates an evaluation that returns a result you can inspect programmatically.
// Unlike Assert, it does not fail a test — you decide what to do with the result.
func Eval(output any) *EvalBuilder {
	return &EvalBuilder{output: output}
}

// Compare creates an A/B comparison of two outputs against the same criteria.
func Compare(a, b any) *CompareBuilder {
	return &CompareBuilder{outputA: a, outputB: b}
}
