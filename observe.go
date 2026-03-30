package sense

import "time"

// Event is emitted after every API call when a Hook or Logger is configured.
type Event struct {
	Op       string        // "eval", "compare", "extract", "extract_slice"
	Model    string        // Model used for the call
	Duration time.Duration // Wall-clock time of the call
	Tokens   int           // Total tokens (input + output)
	Usage    *Usage        // Detailed token breakdown
	Err      error         // Non-nil if the call failed
}
