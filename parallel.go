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

			if err := validateDest(j.Dest); err != nil {
				errs[idx] = err
				return
			}

			schema := schemaForValue(j.Dest)

			extCtx := j.Context
			if s.context != "" {
				if extCtx != "" {
					extCtx = s.context + "\n" + extCtx
				} else {
					extCtx = s.context
				}
			}
			userMsg := buildExtractUserMessage(j.Text, extCtx)

			model := j.Model
			if model == "" {
				model = s.getModel()
			}

			callCtx := ctx
			if s.timeout > 0 {
				var cancel context.CancelFunc
				callCtx, cancel = context.WithTimeout(ctx, s.timeout)
				defer cancel()
			}

			raw, usage, err := s.client.call(callCtx, &callRequest{
				systemPrompt: extractSystemPrompt,
				userMessage:  userMsg,
				toolName:     "submit_extraction",
				toolSchema:   schema,
				model:        model,
			})
			s.recordUsage(usage)
			if err != nil {
				errs[idx] = &Error{Op: "extract", Message: "api call failed", Err: err}
				return
			}

			if err := json.Unmarshal(raw, j.Dest); err != nil {
				errs[idx] = &Error{Op: "extract", Message: "failed to parse result", Err: err}
				return
			}
		}(i, job)
	}

	wg.Wait()
	return &ExtractParallelResult{
		Errors:   errs,
		Duration: time.Since(start),
	}
}
