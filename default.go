package sense

import (
	"sync"
	"testing"
)

var (
	defaultOnce sync.Once
	defaultSess *Session //nolint:gochecknoglobals // lazy singleton for zero-config API
)

func getDefault() *Session {
	defaultOnce.Do(func() {
		defaultSess = New()
	})
	return defaultSess
}

// Assert creates a test assertion using the default session.
// The output can be a string, []byte, or any type (structs are serialized to JSON).
// Calls t.Error() on failure (test continues).
//
//	sense.Assert(t, output).Expect("covers all sections").Run()
func Assert(t testing.TB, output any) *AssertBuilder {
	return getDefault().Assert(t, output)
}

// Require creates a test assertion using the default session.
// Calls t.Fatal() on failure (test stops).
//
//	sense.Require(t, output).Expect("is valid JSON").Run()
func Require(t testing.TB, output any) *AssertBuilder {
	return getDefault().Require(t, output)
}

// Eval creates an evaluation using the default session.
// Returns a result you can inspect programmatically.
//
//	result, err := sense.Eval(output).Expect("under 200 words").Judge()
func Eval(output any) *EvalBuilder {
	return getDefault().Eval(output)
}

// Compare creates an A/B comparison using the default session.
//
//	result, err := sense.Compare(a, b).Criteria("readability").Judge()
func Compare(a, b any) *CompareBuilder {
	return getDefault().Compare(a, b)
}
