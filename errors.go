package sense

import (
	"errors"
	"fmt"
)

var (
	// ErrNoAPIKey is returned when no API key is configured.
	ErrNoAPIKey = errors.New("sense: no API key configured (set ANTHROPIC_API_KEY or call Configure)")

	// ErrRateLimit is returned when the API rate limits the request after all retries.
	ErrRateLimit = errors.New("sense: rate limited after all retries")

	// ErrTimeout is returned when the request exceeds the configured timeout.
	ErrTimeout = errors.New("sense: request timed out")

	// ErrNoToolCall is returned when the model response contains no tool call.
	ErrNoToolCall = errors.New("sense: model did not produce a tool call")

	// ErrNoExpectations is returned when Judge/Run is called with no Expect() calls.
	ErrNoExpectations = errors.New("sense: no expectations provided (call Expect() at least once)")

	// ErrSkipped is returned when SENSE_SKIP=1 is set.
	ErrSkipped = errors.New("sense: skipped (SENSE_SKIP=1)")
)

// Error wraps an underlying error with AgentKit context.
type Error struct {
	Op      string // Operation that failed (e.g., "eval", "assert")
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("sense %s: %s: %v", e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("sense %s: %s", e.Op, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}
