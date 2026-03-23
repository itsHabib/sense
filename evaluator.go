package sense

import "testing"

// Evaluator is the interface for judging non-deterministic output.
// Use it to type function parameters that accept a sense session,
// decoupling your code from the concrete Session struct.
//
//	func AnalyzeReport(s sense.Evaluator, doc string) error {
//	    result, err := s.Eval(doc).
//	        Expect("has executive summary").
//	        Judge()
//	    // ...
//	}
type Evaluator interface {
	Assert(t testing.TB, output any) *AssertBuilder
	Require(t testing.TB, output any) *AssertBuilder
	Eval(output any) *EvalBuilder
	Compare(a, b any) *CompareBuilder
}

var _ Evaluator = (*Session)(nil)
