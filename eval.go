package sense

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// EvalResult is the structured result of an evaluation.
type EvalResult struct {
	Pass       bool          `json:"pass"`
	Score      float64       `json:"score"`
	Checks     []Check       `json:"checks"`
	Duration   time.Duration `json:"-"`
	TokensUsed int           `json:"-"`
	Model      string        `json:"-"`
	Usage      Usage         `json:"-"`
}

// Check is a single expectation evaluation.
type Check struct {
	Expect         string  `json:"expect"`
	Pass           bool    `json:"pass"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason"`
	Evidence       string  `json:"evidence,omitempty"`
	BelowThreshold bool    `json:"below_threshold,omitempty"`
}

// FailedChecks returns only the checks that failed.
func (r *EvalResult) FailedChecks() []Check {
	var failed []Check
	for _, c := range r.Checks {
		if !c.Pass {
			failed = append(failed, c)
		}
	}
	return failed
}

// PassedChecks returns only the checks that passed.
func (r *EvalResult) PassedChecks() []Check {
	var passed []Check
	for _, c := range r.Checks {
		if c.Pass {
			passed = append(passed, c)
		}
	}
	return passed
}

// String returns a human-readable summary of the evaluation.
func (r *EvalResult) String() string {
	passed := len(r.PassedChecks())
	total := len(r.Checks)

	var b strings.Builder
	fmt.Fprintf(&b, "evaluation: %d/%d passed, score: %.2f\n", passed, total, r.Score)

	for _, c := range r.Checks {
		switch {
		case c.BelowThreshold, !c.Pass:
			fmt.Fprintf(&b, "\n    \u2717 %s\n", c.Expect)
		default:
			fmt.Fprintf(&b, "\n    \u2713 %s\n", c.Expect)
		}
		fmt.Fprintf(&b, "      reason: %s\n", c.Reason)
		if c.Evidence != "" {
			fmt.Fprintf(&b, "      evidence: %s\n", c.Evidence)
		}
		if c.BelowThreshold {
			fmt.Fprintf(&b, "      confidence: %.2f (below threshold)\n", c.Confidence)
		} else {
			fmt.Fprintf(&b, "      confidence: %.2f\n", c.Confidence)
		}
	}

	return b.String()
}

// EvalBuilder constructs and executes an evaluation.
type EvalBuilder struct {
	session      *Session
	output       any
	expectations []string
	context      string
	model        string
	timeout      time.Duration
	timeoutSet   bool

	minConfidence    float64
	minConfidenceSet bool
}

// Expect adds a natural language expectation. Chainable.
func (b *EvalBuilder) Expect(expectation string) *EvalBuilder {
	b.expectations = append(b.expectations, expectation)
	return b
}

// Context adds background information for the judge.
func (b *EvalBuilder) Context(ctx string) *EvalBuilder {
	b.context = ctx
	return b
}

// Model overrides the judge model for this evaluation.
func (b *EvalBuilder) Model(model string) *EvalBuilder {
	b.model = model
	return b
}

// Timeout overrides the per-call timeout for this evaluation.
func (b *EvalBuilder) Timeout(d time.Duration) *EvalBuilder {
	b.timeout = d
	b.timeoutSet = true
	return b
}

// MinConfidence sets the minimum confidence threshold for this evaluation.
// Checks that pass Claude's judgment but fall below this threshold are
// treated as failures. Overrides the session-level WithMinConfidence default.
func (b *EvalBuilder) MinConfidence(threshold float64) *EvalBuilder {
	b.minConfidence = threshold
	b.minConfidenceSet = true
	return b
}

// Judge executes the evaluation and returns the result.
func (b *EvalBuilder) Judge() (*EvalResult, error) {
	return b.JudgeContext(context.Background())
}

// JudgeContext executes the evaluation with the given context.
func (b *EvalBuilder) JudgeContext(ctx context.Context) (*EvalResult, error) {
	if len(b.expectations) == 0 {
		return nil, ErrNoExpectations
	}

	if shouldSkip() {
		checks := make([]Check, len(b.expectations))
		for i, exp := range b.expectations {
			checks[i] = Check{Expect: exp, Pass: true, Confidence: 1.0, Reason: "skipped (SENSE_SKIP=1)"}
		}
		return &EvalResult{Pass: true, Score: 1.0, Checks: checks}, nil
	}

	outputStr := serializeOutput(b.output)

	userMsg := buildEvalUserMessage(outputStr, b.expectations, mergeContext(b.session.context, b.context))

	if timeout := resolveTimeout(b.timeout, b.timeoutSet, b.session.timeout); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	start := time.Now()

	model := b.model
	if model == "" {
		model = b.session.getModel()
	}

	raw, usage, err := b.session.client.call(ctx, &callRequest{
		systemPrompt: evalSystemPrompt,
		userMessage:  userMsg,
		toolName:     "submit_evaluation",
		toolSchema:   evalToolSchema,
		model:        model,
	})
	b.session.recordUsage(usage)
	if err != nil {
		return nil, &Error{Op: "eval", Message: "api call failed", Err: err}
	}

	var result EvalResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, &Error{Op: "eval", Message: "failed to parse result", Err: err}
	}

	result.Duration = time.Since(start)
	result.Model = model
	if usage != nil {
		result.TokensUsed = usage.InputTokens + usage.OutputTokens
		result.Usage = *usage
	}

	threshold := b.minConfidence
	if !b.minConfidenceSet {
		threshold = b.session.minConfidence
	}
	applyConfidenceThreshold(&result, threshold)

	b.session.emit(Event{
		Op:       "eval",
		Model:    model,
		Duration: result.Duration,
		Tokens:   result.TokensUsed,
		Usage:    usage,
	})

	return &result, nil
}

// applyConfidenceThreshold marks low-confidence passes as below threshold
// and recalculates Pass/Score accordingly.
func applyConfidenceThreshold(result *EvalResult, threshold float64) {
	if threshold <= 0 {
		return
	}
	allPass := true
	passCount := 0
	for i := range result.Checks {
		if result.Checks[i].Pass && result.Checks[i].Confidence < threshold {
			result.Checks[i].BelowThreshold = true
			allPass = false
		} else if result.Checks[i].Pass {
			passCount++
		}
	}
	result.Pass = allPass
	if len(result.Checks) > 0 {
		result.Score = float64(passCount) / float64(len(result.Checks))
	}
}
