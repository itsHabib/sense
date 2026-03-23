// Package sense provides two tools for working with unstructured text,
// powered by Claude.
//
// Extract parses messy text into typed Go structs — logs, error messages,
// support tickets, API responses. Define a struct, get structured data back.
//
// Judge evaluates non-deterministic output against expectations. Assert in
// tests, eval programmatically, A/B compare two outputs.
//
// Both use the Anthropic API with forced tool_use for structured responses.
// No prompt engineering, no JSON parsing — the schema is enforced server-side.
//
//	s := sense.NewSession(sense.Config{})
//	defer s.Close()
//
//	// Extract: unstructured text → typed struct
//	var m MountError
//	s.Extract("device /dev/sdf already mounted", &m).Run()
//
//	// Judge: output → pass/fail with evidence
//	s.Assert(t, doc).
//	    Expect("covers all sections from the brief").
//	    Run()
package sense

import "testing"

// Assert creates a test assertion that calls t.Error() on failure (test continues).
// The output can be a string, []byte, or any type (structs are serialized to JSON).
func (s *Session) Assert(t testing.TB, output any) *AssertBuilder {
	return &AssertBuilder{
		t:     t,
		eval:  &EvalBuilder{session: s, output: output},
		fatal: false,
	}
}

// Require creates a test assertion that calls t.Fatal() on failure (test stops).
// The output can be a string, []byte, or any type (structs are serialized to JSON).
func (s *Session) Require(t testing.TB, output any) *AssertBuilder {
	return &AssertBuilder{
		t:     t,
		eval:  &EvalBuilder{session: s, output: output},
		fatal: true,
	}
}

// Eval creates an evaluation that returns a result you can inspect programmatically.
// Unlike Assert, it does not fail a test — you decide what to do with the result.
func (s *Session) Eval(output any) *EvalBuilder {
	return &EvalBuilder{session: s, output: output}
}

// Compare creates an A/B comparison of two outputs against the same criteria.
func (s *Session) Compare(a, b any) *CompareBuilder {
	return &CompareBuilder{session: s, outputA: a, outputB: b}
}
