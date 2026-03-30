package sense

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

// Extractor is the interface for extracting structured data from text.
// Use it to type function parameters that accept a sense session,
// decoupling your code from the concrete Session struct.
//
//	func ParseTicket(s sense.Extractor, raw string) (*Ticket, error) {
//	    var t Ticket
//	    _, err := s.Extract(raw, &t).Run()
//	    return &t, err
//	}
type Extractor interface {
	Extract(text string, dest any) *ExtractIntoBuilder
}

var _ Extractor = (*Session)(nil)

// Extract creates a builder that extracts structured data from text into dest.
// dest must be a non-nil pointer to a struct. Use json tags for field names
// and sense tags for descriptions:
//
//	type MountError struct {
//	    Device   string `json:"device" sense:"The device path"`
//	    VolumeID string `json:"volume_id" sense:"The EBS volume ID"`
//	}
//
//	var m MountError
//	_, err := s.Extract("device /dev/sdf already mounted with vol-123", &m).Run()
func (s *Session) Extract(text string, dest any) *ExtractIntoBuilder {
	return &ExtractIntoBuilder{
		session: s,
		text:    text,
		dest:    dest,
	}
}

// Validator is an optional interface that extraction destination structs
// can implement. If dest implements Validator, sense calls Validate()
// automatically after unmarshalling.
type Validator interface {
	Validate() error
}

// ExtractIntoBuilder constructs and executes a structured extraction
// using the json.Unmarshal pattern. The extracted data is written
// directly into the dest pointer passed to Extract.
type ExtractIntoBuilder struct {
	session    *Session
	text       string
	dest       any
	context    string
	model      string
	timeout    time.Duration
	timeoutSet bool
	fallback   func() error
}

// Context adds background information to guide extraction. Chainable.
func (b *ExtractIntoBuilder) Context(ctx string) *ExtractIntoBuilder {
	b.context = ctx
	return b
}

// Model overrides the model for this extraction. Chainable.
func (b *ExtractIntoBuilder) Model(model string) *ExtractIntoBuilder {
	b.model = model
	return b
}

// Timeout overrides the per-call timeout for this extraction. Chainable.
func (b *ExtractIntoBuilder) Timeout(d time.Duration) *ExtractIntoBuilder {
	b.timeout = d
	b.timeoutSet = true
	return b
}

// Fallback sets a function to call when extraction fails. The function
// should populate the dest pointer directly. Chainable.
func (b *ExtractIntoBuilder) Fallback(fn func() error) *ExtractIntoBuilder {
	b.fallback = fn
	return b
}

// Run executes the extraction and returns the result.
// The extracted data is written into the dest pointer passed to Extract.
func (b *ExtractIntoBuilder) Run() (*ExtractIntoResult, error) {
	return b.RunContext(context.Background())
}

// RunContext executes the extraction with the given context.
// The extracted data is written into the dest pointer passed to Extract.
func (b *ExtractIntoBuilder) RunContext(ctx context.Context) (*ExtractIntoResult, error) {
	if b.text == "" {
		return nil, ErrNoText
	}

	if err := validateDest(b.dest); err != nil {
		return nil, err
	}

	if shouldSkip() {
		return &ExtractIntoResult{}, nil
	}

	schema := schemaForValue(b.dest)

	userMsg := buildExtractUserMessage(b.text, mergeContext(b.session.context, b.context))

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
		toolSchema:   schema,
		model:        model,
	})
	b.session.recordUsage(usage)
	if err != nil {
		b.session.emit(Event{Op: "extract", Model: model, Duration: time.Since(start), Err: err})
		if b.fallback != nil {
			if fbErr := b.fallback(); fbErr != nil {
				return nil, &Error{Op: "extract", Message: "fallback failed", Err: fbErr}
			}
			return &ExtractIntoResult{Duration: time.Since(start), Model: model}, nil
		}
		return nil, &Error{Op: "extract", Message: "api call failed", Err: err}
	}

	if err := json.Unmarshal(raw, b.dest); err != nil {
		return nil, &Error{Op: "extract", Message: "failed to parse result", Err: err}
	}

	if v, ok := b.dest.(Validator); ok {
		if err := v.Validate(); err != nil {
			return nil, &Error{Op: "extract", Message: "validation failed", Err: err}
		}
	}

	result := &ExtractIntoResult{
		Duration: time.Since(start),
		Model:    model,
	}
	if usage != nil {
		result.TokensUsed = usage.InputTokens + usage.OutputTokens
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

// ExtractIntoResult holds metadata from an extraction. The extracted data
// is in the dest pointer passed to Extract.
type ExtractIntoResult struct {
	Duration   time.Duration
	TokensUsed int
	Model      string
}

// validateDest checks that dest is a non-nil pointer to a struct.
func validateDest(dest any) error {
	if dest == nil {
		return &Error{Op: "extract", Message: "dest must be a non-nil pointer to a struct"}
	}
	t := reflect.TypeOf(dest)
	if t.Kind() != reflect.Ptr {
		return &Error{Op: "extract", Message: fmt.Sprintf("dest must be a pointer, got %s", t.Kind())}
	}
	if t.Elem().Kind() != reflect.Struct {
		return &Error{Op: "extract", Message: fmt.Sprintf("dest must point to a struct, got pointer to %s", t.Elem().Kind())}
	}
	return nil
}
