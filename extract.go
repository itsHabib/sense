package sense

import (
	"context"
	"encoding/json"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// ExtractResult is the structured result of an extraction.
type ExtractResult[T any] struct {
	Data       T             `json:"-"`
	Duration   time.Duration `json:"-"`
	TokensUsed int           `json:"-"`
	Model      string        `json:"-"`
}

// ExtractBuilder constructs and executes a structured extraction.
type ExtractBuilder[T any] struct {
	session *Session
	text    string
	context string
	model   string
	schema  anthropic.ToolInputSchemaParam
}

// Extract creates a builder that extracts structured data from text into type T.
// This is the generic variant with compile-time type safety. For the method
// variant that works with the [Extractor] interface, use [Session.Extract].
//
// T must be a struct with exported fields. Use json tags for field names
// and sense tags for descriptions:
//
//	type MountError struct {
//	    Device   string `json:"device" sense:"The device path"`
//	    VolumeID string `json:"volume_id" sense:"The EBS volume ID"`
//	}
//
//	result, err := sense.Extract[MountError](s, "device /dev/sdf already mounted with vol-123").Run()
func Extract[T any](s *Session, text string) *ExtractBuilder[T] {
	return &ExtractBuilder[T]{
		session: s,
		text:    text,
		schema:  schemaFor[T](),
	}
}

// Context adds background information to guide extraction. Chainable.
func (b *ExtractBuilder[T]) Context(ctx string) *ExtractBuilder[T] {
	b.context = ctx
	return b
}

// Model overrides the model for this extraction. Chainable.
func (b *ExtractBuilder[T]) Model(model string) *ExtractBuilder[T] {
	b.model = model
	return b
}

// Run executes the extraction and returns the result.
func (b *ExtractBuilder[T]) Run() (*ExtractResult[T], error) {
	return b.RunContext(context.Background())
}

// RunContext executes the extraction with the given context.
func (b *ExtractBuilder[T]) RunContext(ctx context.Context) (*ExtractResult[T], error) {
	if b.text == "" {
		return nil, ErrNoText
	}

	if shouldSkip() {
		var zero T
		return &ExtractResult[T]{Data: zero}, nil
	}

	userMsg := buildExtractUserMessage(b.text, b.context)

	timeout := b.session.timeout
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
		return nil, &Error{Op: "extract", Message: "api call failed", Err: err}
	}

	var data T
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, &Error{Op: "extract", Message: "failed to parse result", Err: err}
	}

	result := &ExtractResult[T]{
		Data:     data,
		Duration: time.Since(start),
		Model:    model,
	}
	if usage != nil {
		result.TokensUsed = usage.InputTokens + usage.OutputTokens
	}

	return result, nil
}
