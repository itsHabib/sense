package sense

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// BatchConfig controls request batching behavior.
type BatchConfig struct {
	MaxSize int           // flush when this many requests are pending
	MaxWait time.Duration // flush after this duration, whichever comes first
}

// batchResult is sent back to each caller via their channel.
type batchResult struct {
	raw   json.RawMessage
	usage *Usage
	err   error
}

// pendingRequest pairs a request with the channel to send its result on.
type pendingRequest struct {
	id   string
	req  *callRequest
	resp chan batchResult
}

// batcher collects requests and flushes them as a single batch API call.
type batcher struct {
	config BatchConfig
	client anthropic.Client

	once  sync.Once
	inbox chan pendingRequest
	stop  chan struct{}

	idCounter atomic.Int64
}

func newBatcher(cfg BatchConfig, apiKey string) *batcher {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &batcher{
		config: cfg,
		client: anthropic.NewClient(opts...),
		inbox:  make(chan pendingRequest),
		stop:   make(chan struct{}),
	}
}

// run is the background goroutine that owns the pending slice.
// pending and timer are local — no mutex needed.
func (b *batcher) run() {
	ctx := context.Background()
	var pending []pendingRequest
	timer := time.NewTimer(b.config.MaxWait)
	timer.Stop()

	for {
		select {
		case req := <-b.inbox:
			pending = append(pending, req)
			if len(pending) == 1 {
				timer.Reset(b.config.MaxWait)
			}
			if len(pending) >= b.config.MaxSize {
				timer.Stop()
				b.flush(ctx, pending)
				pending = nil
			}

		case <-timer.C:
			if len(pending) > 0 {
				b.flush(ctx, pending)
				pending = nil
			}

		case <-b.stop:
			timer.Stop()
			if len(pending) > 0 {
				b.flush(ctx, pending)
			}
			return
		}
	}
}

// submitRequest sends a request to the batcher and blocks until the result arrives.
func (b *batcher) submitRequest(req *callRequest) (json.RawMessage, *Usage, error) {
	b.once.Do(func() { go b.run() })

	ch := make(chan batchResult, 1)
	id := fmt.Sprintf("req-%d", b.idCounter.Add(1))

	b.inbox <- pendingRequest{id: id, req: req, resp: ch}

	result := <-ch
	return result.raw, result.usage, result.err
}

// flush submits pending requests as a batch, polls until done, fans out results.
func (b *batcher) flush(ctx context.Context, pending []pendingRequest) {
	params := buildBatchParams(pending)

	batch, err := b.client.Messages.Batches.New(ctx, params)
	if err != nil {
		fanOutError(pending, fmt.Errorf("batch submit failed: %w", err))
		return
	}

	if _, err = b.pollUntilDone(ctx, batch.ID); err != nil {
		fanOutError(pending, fmt.Errorf("batch poll failed: %w", err))
		return
	}

	b.fanOutResults(ctx, batch.ID, pending)
}

// buildBatchParams converts pending requests to SDK params.
func buildBatchParams(pending []pendingRequest) anthropic.MessageBatchNewParams {
	reqs := make([]anthropic.MessageBatchNewParamsRequest, len(pending))
	for i, p := range pending {
		reqs[i] = anthropic.MessageBatchNewParamsRequest{
			CustomID: p.id,
			Params: anthropic.MessageBatchNewParamsRequestParams{
				Model:     p.req.model,
				MaxTokens: 4096,
				System: []anthropic.TextBlockParam{
					{Text: p.req.systemPrompt},
				},
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock(p.req.userMessage)),
				},
				Tools: []anthropic.ToolUnionParam{{
					OfTool: &anthropic.ToolParam{
						Name:        p.req.toolName,
						Description: anthropic.String("Submit the structured result"),
						InputSchema: p.req.toolSchema,
					},
				}},
				ToolChoice: anthropic.ToolChoiceParamOfTool(p.req.toolName),
			},
		}
	}
	return anthropic.MessageBatchNewParams{Requests: reqs}
}

// pollUntilDone polls the batch status with exponential backoff (1s→30s).
func (b *batcher) pollUntilDone(ctx context.Context, batchID string) (*anthropic.MessageBatch, error) {
	interval := 1 * time.Second
	maxInterval := 30 * time.Second

	for {
		batch, err := b.client.Messages.Batches.Get(ctx, batchID)
		if err != nil {
			return nil, err
		}

		if batch.ProcessingStatus == anthropic.MessageBatchProcessingStatusEnded {
			return batch, nil
		}
		// "canceling" — keep polling, it'll end eventually.

		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}

		interval = min(interval*2, maxInterval)
	}
}

// fanOutResults streams batch results and sends each to the correct caller.
func (b *batcher) fanOutResults(ctx context.Context, batchID string, pending []pendingRequest) {
	lookup := make(map[string]chan batchResult, len(pending))
	for _, p := range pending {
		lookup[p.id] = p.resp
	}

	stream := b.client.Messages.Batches.ResultsStreaming(ctx, batchID)
	for stream.Next() {
		item := stream.Current() //nolint:gocritic // SDK returns value, can't avoid copy
		ch, ok := lookup[item.CustomID]
		if !ok {
			continue
		}

		deliverResult(item, ch)
		delete(lookup, item.CustomID)
	}

	if err := stream.Err(); err != nil {
		for _, ch := range lookup {
			ch <- batchResult{err: fmt.Errorf("result stream failed: %w", err)}
		}
		return
	}

	for id, ch := range lookup {
		ch <- batchResult{err: fmt.Errorf("no result for request %s", id)}
	}
}

// deliverResult sends a single batch result item to the caller's channel.
//
//nolint:gocritic // SDK types are large structs; we receive them by value from the SDK stream.
func deliverResult(item anthropic.MessageBatchIndividualResponse, ch chan batchResult) {
	switch item.Result.Type {
	case "succeeded":
		msg := item.Result.AsSucceeded().Message
		raw, usage := extractBatchToolUse(msg)
		ch <- batchResult{raw: raw, usage: usage}
	case "errored":
		ch <- batchResult{err: fmt.Errorf("batch request failed: %v", item.Result.AsErrored().Error)}
	case "canceled":
		ch <- batchResult{err: errors.New("batch request was canceled")}
	case "expired":
		ch <- batchResult{err: errors.New("batch request expired")}
	}
}

// extractBatchToolUse pulls tool call JSON from a batch result Message.
//
//nolint:gocritic // SDK type; received by value from the SDK.
func extractBatchToolUse(msg anthropic.Message) (json.RawMessage, *Usage) {
	usage := &Usage{
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
	}
	for i := range msg.Content {
		if variant, ok := msg.Content[i].AsAny().(anthropic.ToolUseBlock); ok {
			return json.RawMessage(variant.JSON.Input.Raw()), usage
		}
	}
	return nil, usage
}

// fanOutError sends the same error to all waiting callers.
func fanOutError(pending []pendingRequest, err error) {
	for _, p := range pending {
		p.resp <- batchResult{err: err}
	}
}

// batchCaller implements the caller interface using the batcher.
type batchCaller struct {
	batcher *batcher
}

func (c *batchCaller) call(_ context.Context, req *callRequest) (json.RawMessage, *Usage, error) {
	// The batcher owns its own context (background) because the batch lifecycle is
	// decoupled from individual caller contexts. Callers block on their response channel.
	return c.batcher.submitRequest(req) //nolint:contextcheck // batch owns its own context; callers block on response channel
}

// close signals the batcher to flush remaining requests and stop.
func (b *batcher) close() {
	close(b.stop)
}
