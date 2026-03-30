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
	Usage      Usage         `json:"-"`
	Fallback   bool          `json:"-"`
}

// ExtractBuilder constructs and executes a structured extraction.
type ExtractBuilder[T any] struct {
	session    *Session
	text       string
	context    string
	model      string
	schema     anthropic.ToolInputSchemaParam
	timeout    time.Duration
	timeoutSet bool
	validate   func(T) error
	fallback   func() (*T, error)
}

// Extract creates a builder that extracts structured data from text into type T
// using the default session. For an explicit session, use [Session.Extract].
//
// T must be a struct with exported fields. Use json tags for field names
// and sense tags for descriptions:
//
//	type MountError struct {
//	    Device   string `json:"device" sense:"The device path"`
//	    VolumeID string `json:"volume_id" sense:"The EBS volume ID"`
//	}
//
//	result, err := sense.Extract[MountError]("device /dev/sdf already mounted with vol-123").Run()
func Extract[T any](text string) *ExtractBuilder[T] {
	return newExtractBuilder[T](getDefault(), text)
}

func newExtractBuilder[T any](s *Session, text string) *ExtractBuilder[T] {
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

// Timeout overrides the per-call timeout for this extraction.
// Set to -1 or 0 to disable timeouts. Chainable.
func (b *ExtractBuilder[T]) Timeout(d time.Duration) *ExtractBuilder[T] {
	b.timeout = d
	b.timeoutSet = true
	return b
}

// Validate sets a function that runs on the extracted data after extraction.
// If validation fails, Run returns an error. Chainable.
func (b *ExtractBuilder[T]) Validate(fn func(T) error) *ExtractBuilder[T] {
	b.validate = fn
	return b
}

// Fallback sets a function to call when extraction fails. If the primary
// extraction returns an error, the fallback is attempted instead. Chainable.
func (b *ExtractBuilder[T]) Fallback(fn func() (*T, error)) *ExtractBuilder[T] {
	b.fallback = fn
	return b
}

// Run executes the extraction and returns the result.
func (b *ExtractBuilder[T]) Run() (*ExtractResult[T], error) {
	return b.RunContext(context.Background())
}

// RunContext executes the extraction with the given context.
func (b *ExtractBuilder[T]) RunContext(ctx context.Context) (*ExtractResult[T], error) {
	if shouldSkip() {
		var zero T
		return &ExtractResult[T]{Data: zero}, nil
	}

	if b.text == "" {
		return nil, ErrNoText
	}

	userMsg := buildExtractUserMessage(b.text, b.session.mergeContext(b.context))

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
		systemPrompt: extractSystemPrompt,
		userMessage:  userMsg,
		toolName:     "submit_extraction",
		toolSchema:   b.schema,
		model:        model,
	})
	b.session.recordUsage(usage)
	if err != nil {
		b.session.emit(Event{Op: "extract", Model: model, Duration: time.Since(start), Err: err})
		if b.fallback != nil {
			data, fbErr := b.fallback()
			if fbErr != nil {
				return nil, &Error{Op: "extract", Message: "fallback failed", Err: fbErr}
			}
			return &ExtractResult[T]{Data: *data, Duration: time.Since(start), Model: model, Fallback: true}, nil
		}
		return nil, &Error{Op: "extract", Message: "api call failed", Err: err}
	}

	var data T
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, &Error{Op: "extract", Message: "failed to parse result", Err: err}
	}

	if b.validate != nil {
		if err := b.validate(data); err != nil {
			return nil, &Error{Op: "extract", Message: "validation failed", Err: err}
		}
	}

	if err := checkValidator(&data); err != nil {
		return nil, &Error{Op: "extract", Message: "validation failed", Err: err}
	}

	result := &ExtractResult[T]{
		Data:     data,
		Duration: time.Since(start),
		Model:    model,
	}
	if usage != nil {
		result.TokensUsed = usage.InputTokens + usage.OutputTokens
		result.Usage = *usage
	}

	b.session.emit(Event{
		Op:       "extract",
		Model:    model,
		Duration: result.Duration,
		Tokens:   result.TokensUsed,
		Usage:    usage,
	})

	return result, nil
}
