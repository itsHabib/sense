package sense

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// ExtractJob describes a single extraction to run in parallel.
type ExtractJob struct {
	Text    string // Text to extract from
	Dest    any    // Pointer to destination struct
	Context string // Optional per-job context
	Model   string // Optional per-job model override
}

// ExtractParallelResult holds the results from a parallel extraction.
type ExtractParallelResult struct {
	Errors   []error       // One entry per job, nil on success
	Duration time.Duration // Total wall-clock time
}

// Failed returns true if any job failed.
func (r *ExtractParallelResult) Failed() bool {
	for _, err := range r.Errors {
		if err != nil {
			return true
		}
	}
	return false
}

// ExtractParallel runs multiple extractions concurrently and returns when
// all complete. Each job's result is written into its Dest pointer.
func (s *Session) ExtractParallel(ctx context.Context, jobs []ExtractJob) *ExtractParallelResult {
	start := time.Now()
	errs := make([]error, len(jobs))
	var wg sync.WaitGroup

	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, j ExtractJob) {
			defer wg.Done()
			errs[idx] = s.runExtractJob(ctx, j)
		}(i, job)
	}

	wg.Wait()
	return &ExtractParallelResult{
		Errors:   errs,
		Duration: time.Since(start),
	}
}

func (s *Session) runExtractJob(ctx context.Context, j ExtractJob) error {
	if err := validateDest(j.Dest); err != nil {
		return err
	}

	schema := schemaForValue(j.Dest)
	userMsg := buildExtractUserMessage(j.Text, mergeContext(s.context, j.Context))

	model := j.Model
	if model == "" {
		model = s.getModel()
	}

	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	raw, usage, err := s.client.call(ctx, &callRequest{
		systemPrompt: extractSystemPrompt,
		userMessage:  userMsg,
		toolName:     "submit_extraction",
		toolSchema:   schema,
		model:        model,
	})
	s.recordUsage(usage)
	if err != nil {
		return &Error{Op: "extract", Message: "api call failed", Err: err}
	}

	if err := json.Unmarshal(raw, j.Dest); err != nil {
		return &Error{Op: "extract", Message: "failed to parse result", Err: err}
	}

	return nil
}
