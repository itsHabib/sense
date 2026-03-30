package sense

import (
	"errors"
	"fmt"
)

var (
	// ErrRateLimit is returned when the API rate limits the request after all retries.
	ErrRateLimit = errors.New("sense: rate limited after all retries")

	// ErrTimeout is returned when a call exceeds its deadline.
	ErrTimeout = errors.New("sense: request timed out")

	// ErrNoToolCall is returned when the model response contains no tool call.
	ErrNoToolCall = errors.New("sense: model did not produce a tool call")

	// ErrNoExpectations is returned when Judge/Run is called with no Expect calls.
	ErrNoExpectations = errors.New("sense: no expectations provided (call Expect at least once)")

	// ErrNoCriteria is returned when Judge is called with no Criteria calls.
	ErrNoCriteria = errors.New("sense: no criteria provided (call Criteria at least once)")

	// ErrNoText is returned when Extract is called with empty text.
	ErrNoText = errors.New("sense: no text provided for extraction")

	// ErrBudgetExceeded is returned when the session cost budget has been exhausted.
	ErrBudgetExceeded = errors.New("sense: cost budget exceeded")
)

// Error wraps an underlying error with operation context.
type Error struct {
	Op      string // Operation that failed: "eval", "compare", "extract"
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
