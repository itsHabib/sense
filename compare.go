package sense

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CompareResult is the result of an A/B comparison.
type CompareResult struct {
	Winner     string            `json:"winner"`
	ScoreA     float64           `json:"score_a"`
	ScoreB     float64           `json:"score_b"`
	Criteria   []CriterionResult `json:"criteria"`
	Reasoning  string            `json:"reasoning"`
	Duration   time.Duration     `json:"-"`
	TokensUsed int               `json:"-"`
	Model      string            `json:"-"`
	Usage      Usage             `json:"-"`
}

// CriterionResult is the result for a single comparison criterion.
type CriterionResult struct {
	Name   string  `json:"name"`
	ScoreA float64 `json:"score_a"`
	ScoreB float64 `json:"score_b"`
	Winner string  `json:"winner"`
	Reason string  `json:"reason"`
}

// String returns a human-readable summary of the comparison.
func (r *CompareResult) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "comparison: winner=%s (A: %.2f, B: %.2f)\n", r.Winner, r.ScoreA, r.ScoreB)
	for _, c := range r.Criteria {
		fmt.Fprintf(&b, "    %s: A=%.2f B=%.2f winner=%s — %s\n", c.Name, c.ScoreA, c.ScoreB, c.Winner, c.Reason)
	}
	if r.Reasoning != "" {
		fmt.Fprintf(&b, "    reasoning: %s\n", r.Reasoning)
	}
	return b.String()
}

// CompareBuilder constructs and executes an A/B comparison.
type CompareBuilder struct {
	session    *Session
	outputA    any
	outputB    any
	criteria   []string
	context    string
	model      string
	timeout    time.Duration
	timeoutSet bool
}

// Criteria adds a comparison dimension. Chainable.
func (b *CompareBuilder) Criteria(criterion string) *CompareBuilder {
	b.criteria = append(b.criteria, criterion)
	return b
}

// Context adds background information for the judge.
func (b *CompareBuilder) Context(ctx string) *CompareBuilder {
	b.context = ctx
	return b
}

// Model overrides the judge model for this comparison.
func (b *CompareBuilder) Model(model string) *CompareBuilder {
	b.model = model
	return b
}

// Timeout overrides the per-call timeout for this comparison.
// Set to -1 or 0 to disable timeouts.
func (b *CompareBuilder) Timeout(d time.Duration) *CompareBuilder {
	b.timeout = d
	b.timeoutSet = true
	return b
}

// Judge executes the comparison.
func (b *CompareBuilder) Judge() (*CompareResult, error) {
	return b.JudgeContext(context.Background())
}

// JudgeContext executes the comparison with the given context.
func (b *CompareBuilder) JudgeContext(ctx context.Context) (*CompareResult, error) {
	if shouldSkip() {
		return &CompareResult{Winner: "tie", ScoreA: 0.5, ScoreB: 0.5, Reasoning: "skipped (SENSE_SKIP=1)"}, nil
	}

	if len(b.criteria) == 0 {
		return nil, ErrNoCriteria
	}

	outputA := serializeOutput(b.outputA)
	outputB := serializeOutput(b.outputB)

	userMsg := buildCompareUserMessage(outputA, outputB, b.criteria, b.session.mergeContext(b.context))

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
		systemPrompt: compareSystemPrompt,
		userMessage:  userMsg,
		toolName:     "submit_comparison",
		toolSchema:   compareToolSchema,
		model:        model,
	})
	b.session.recordUsage(usage)
	if err != nil {
		return nil, &Error{Op: "compare", Message: "api call failed", Err: err}
	}

	var result CompareResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, &Error{Op: "compare", Message: "failed to parse result", Err: err}
	}

	result.Duration = time.Since(start)
	result.Model = model
	if usage != nil {
		result.TokensUsed = usage.InputTokens + usage.OutputTokens
		result.Usage = *usage
	}

	b.session.emit(Event{
		Op:       "compare",
		Model:    model,
		Duration: result.Duration,
		Tokens:   result.TokensUsed,
		Usage:    usage,
	})

	return &result, nil
}
