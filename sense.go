// Package sense makes sense of non-deterministic output.
//
// Evaluate, compare, and extract structured data from text using Claude
// as the judge. Designed for testing agents but works anywhere you need
// to judge or parse unstructured output.
//
//	s := sense.NewSession(sense.Config{})
//	defer s.Close()
//
//	// Test assertions
//	s.Assert(t, doc).
//	    Expect("covers all sections from the brief").
//	    Run()
//
//	// Structured extraction
//	result, err := sense.Extract[MountError](s, "device /dev/sdf already mounted").Run()
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
