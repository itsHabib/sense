package sense

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// ExtractSliceResult is the structured result of a slice extraction.
type ExtractSliceResult[T any] struct {
	Data       []T           `json:"-"`
	Duration   time.Duration `json:"-"`
	TokensUsed int           `json:"-"`
	Model      string        `json:"-"`
}

// ExtractSliceBuilder constructs and executes a structured extraction
// that returns a slice of typed structs from a single text input.
type ExtractSliceBuilder[T any] struct {
	session    *Session
	text       string
	context    string
	model      string
	schema     anthropic.ToolInputSchemaParam
	validate   func(T) error
	timeout    time.Duration
	timeoutSet bool
	fallback   func() ([]T, error)
}

// ExtractSlice creates a builder that extracts a list of typed structs from
// text using the default session.
//
// T must be a struct with exported fields. Use json tags for field names
// and sense tags for descriptions:
//
//	type LogEntry struct {
//	    Level   string `json:"level" sense:"Log level"`
//	    Message string `json:"message"`
//	}
//
//	result, err := sense.ExtractSlice[LogEntry](multiLineLog).Run()
//	for _, entry := range result.Data {
//	    fmt.Println(entry.Level, entry.Message)
//	}
func ExtractSlice[T any](text string) *ExtractSliceBuilder[T] {
	return newExtractSliceBuilder[T](getDefault(), text)
}

func newExtractSliceBuilder[T any](s *Session, text string) *ExtractSliceBuilder[T] {
	return &ExtractSliceBuilder[T]{
		session: s,
		text:    text,
		schema:  sliceSchemaFor[T](),
	}
}

// Context adds background information to guide extraction. Chainable.
func (b *ExtractSliceBuilder[T]) Context(ctx string) *ExtractSliceBuilder[T] {
	b.context = ctx
	return b
}

// Model overrides the model for this extraction. Chainable.
func (b *ExtractSliceBuilder[T]) Model(model string) *ExtractSliceBuilder[T] {
	b.model = model
	return b
}

// Validate sets a function that runs on each extracted item individually.
// If any item fails validation, Run returns an error. Chainable.
func (b *ExtractSliceBuilder[T]) Validate(fn func(T) error) *ExtractSliceBuilder[T] {
	b.validate = fn
	return b
}

// Timeout overrides the per-call timeout for this extraction. Chainable.
func (b *ExtractSliceBuilder[T]) Timeout(d time.Duration) *ExtractSliceBuilder[T] {
	b.timeout = d
	b.timeoutSet = true
	return b
}

// Fallback sets a function to call when extraction fails. Chainable.
func (b *ExtractSliceBuilder[T]) Fallback(fn func() ([]T, error)) *ExtractSliceBuilder[T] {
	b.fallback = fn
	return b
}

// Run executes the extraction and returns the result.
func (b *ExtractSliceBuilder[T]) Run() (*ExtractSliceResult[T], error) {
	return b.RunContext(context.Background())
}

// RunContext executes the extraction with the given context.
func (b *ExtractSliceBuilder[T]) RunContext(ctx context.Context) (*ExtractSliceResult[T], error) {
	if b.text == "" {
		return nil, ErrNoText
	}

	if shouldSkip() {
		return &ExtractSliceResult[T]{Data: []T{}}, nil
	}

	extCtx := b.context
	if b.session.context != "" {
		if extCtx != "" {
			extCtx = b.session.context + "\n" + extCtx
		} else {
			extCtx = b.session.context
		}
	}
	userMsg := buildExtractUserMessage(b.text, extCtx)

	timeout := b.timeout
	if !b.timeoutSet {
		timeout = b.session.timeout
	}
	if timeout > 0 {
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
		systemPrompt: extractSystemPrompt,
		userMessage:  userMsg,
		toolName:     "submit_extraction",
		toolSchema:   b.schema,
		model:        model,
	})
	b.session.recordUsage(usage)
	if err != nil {
		b.session.emit(Event{Op: "extract_slice", Model: model, Duration: time.Since(start), Err: err})
		if b.fallback != nil {
			data, fbErr := b.fallback()
			if fbErr != nil {
				return nil, &Error{Op: "extract_slice", Message: "fallback failed", Err: fbErr}
			}
			return &ExtractSliceResult[T]{Data: data, Duration: time.Since(start), Model: model}, nil
		}
		return nil, &Error{Op: "extract_slice", Message: "api call failed", Err: err}
	}

	var wrapper struct {
		Items []T `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, &Error{Op: "extract_slice", Message: "failed to parse result", Err: err}
	}

	if err := b.validateItems(wrapper.Items); err != nil {
		return nil, err
	}

	result := &ExtractSliceResult[T]{
		Data:     wrapper.Items,
		Duration: time.Since(start),
		Model:    model,
	}
	if usage != nil {
		result.TokensUsed = usage.InputTokens + usage.OutputTokens
	}

	b.session.emit(Event{
		Op:       "extract_slice",
		Model:    model,
		Duration: result.Duration,
		Tokens:   result.TokensUsed,
		Usage:    usage,
	})

	return result, nil
}

func (b *ExtractSliceBuilder[T]) validateItems(items []T) error {
	if b.validate == nil {
		return nil
	}

	for i, item := range items {
		if err := b.validate(item); err != nil {
			return &Error{
				Op:      "extract_slice",
				Message: fmt.Sprintf("validation failed on item %d", i),
				Err:     err,
			}
		}
	}

	return nil
}

// sliceSchemaFor wraps the struct schema for T in an object with an
// "items" array property, so the model returns a list of T.
func sliceSchemaFor[T any]() anthropic.ToolInputSchemaParam {
	inner := schemaFor[T]()

	itemSchema := map[string]any{
		"type":       "object",
		"properties": inner.Properties,
	}
	if len(inner.Required) > 0 {
		itemSchema["required"] = inner.Required
	}

	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"items": map[string]any{
				"type":        "array",
				"items":       itemSchema,
				"description": "List of extracted items",
			},
		},
		Required: []string{"items"},
	}
}
