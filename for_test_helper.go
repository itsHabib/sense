package sense

import "testing"

// ForTest creates a Session scoped to the test lifetime.
// Close is called automatically via t.Cleanup, and a usage summary
// is printed to the test log.
//
//	s := sense.ForTest(t)
//	s.Assert(t, output).Expect("is valid").Run()
func ForTest(t testing.TB, opts ...Option) *Session {
	t.Helper()
	s := New(opts...)
	t.Cleanup(func() {
		u := s.Usage()
		if u.Calls > 0 {
			t.Logf("%s", u.String())
		}
		s.Close()
	})
	return s
}
