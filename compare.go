package sense

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// CompareResult is the result of an A/B comparison.
type CompareResult struct {
	Winner    string            `json:"winner"`
	ScoreA    float64           `json:"score_a"`
	ScoreB    float64           `json:"score_b"`
	Criteria  []CriterionResult `json:"criteria"`
	Reasoning string            `json:"reasoning"`
	Duration  time.Duration     `json:"-"`
}

// CriterionResult is the result for a single comparison criterion.
type CriterionResult struct {
	Name   string  `json:"name"`
	ScoreA float64 `json:"score_a"`
	ScoreB float64 `json:"score_b"`
	Winner string  `json:"winner"`
	Reason string  `json:"reason"`
}

// CompareBuilder constructs and executes an A/B comparison.
type CompareBuilder struct {
	outputA  any
	outputB  any
	criteria []string
	context  string
	model    string
}

// Criteria adds a comparison dimension. Chainable.
func (b *CompareBuilder) Criteria(criterion string) *CompareBuilder {
	b.criteria = append(b.criteria, criterion)
	return b
}

// Context adds background information.
func (b *CompareBuilder) Context(ctx string) *CompareBuilder {
	b.context = ctx
	return b
}

// Model overrides the judge model.
func (b *CompareBuilder) Model(model string) *CompareBuilder {
	b.model = model
	return b
}

// Judge executes the comparison.
func (b *CompareBuilder) Judge() (*CompareResult, error) {
	return b.JudgeContext(context.Background())
}

// JudgeContext executes the comparison with the given context.
func (b *CompareBuilder) JudgeContext(ctx context.Context) (*CompareResult, error) {
	if len(b.criteria) == 0 {
		return nil, errors.New("sense: no criteria provided (call Criteria() at least once)")
	}

	if shouldSkip() {
		return &CompareResult{Winner: "tie", ScoreA: 0.5, ScoreB: 0.5, Reasoning: "skipped (SENSE_SKIP=1)"}, nil
	}

	outputA := serializeOutput(b.outputA)
	outputB := serializeOutput(b.outputB)
	userMsg := buildCompareUserMessage(outputA, outputB, b.criteria, b.context)

	ctx, cancel := context.WithTimeout(ctx, globalConfig.Timeout)
	defer cancel()

	start := time.Now()

	client := getClient()
	raw, _, err := client.call(ctx, callRequest{
		systemPrompt: compareSystemPrompt,
		userMessage:  userMsg,
		toolName:     "submit_comparison",
		toolSchema:   compareToolSchema,
		model:        b.model,
	})
	if err != nil {
		return nil, &Error{Op: "compare", Message: "api call failed", Err: err}
	}

	var result CompareResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, &Error{Op: "compare", Message: "failed to parse comparison result", Err: err}
	}

	result.Duration = time.Since(start)
	return &result, nil
}
