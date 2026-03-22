# Batch Client Design

## What It Is

An internal request batcher for the sense client. Callers call `Judge()` / `Run()` like normal — they block and get results back. Under the hood, the batcher collects requests and submits them as a single Anthropic Batches API call. 50% cost reduction, no API change.

## Caller Perspective

Nothing changes:

```go
// These can run concurrently — each blocks until its result arrives
result, err := sense.Eval(doc).Expect("is valid").Judge()
cmp, err := sense.Compare(a, b).Criteria("quality").Judge()
```

Enable batching via config:

```go
sense.Configure(sense.Config{
    Batch: &sense.BatchConfig{
        MaxSize:   20,                     // flush after 20 requests
        MaxWait:   500 * time.Millisecond, // or flush after 500ms, whichever first
    },
})
```

When `Batch` is nil (default), requests go directly to Claude as individual calls — current behavior, no change.

## SDK Support

The Anthropic Go SDK (v1.27.1) fully supports the Batches API:

```go
// Access via:
client.Messages.Batches.New(ctx, params)              // create batch
client.Messages.Batches.Get(ctx, batchID)             // poll status
client.Messages.Batches.ResultsStreaming(ctx, batchID) // stream results as JSONL
client.Messages.Batches.Cancel(ctx, batchID)          // cancel
```

### Key SDK Types

```go
// Creating a batch
anthropic.MessageBatchNewParams{
    Requests: []anthropic.MessageBatchNewParamsRequest{
        {
            CustomID: "req-1",  // our correlation ID
            Params: anthropic.MessageBatchNewParamsRequestParams{
                Model:      anthropic.ModelClaudeSonnet4_6,
                MaxTokens:  4096,
                Messages:   []anthropic.MessageParam{...},
                System:     []anthropic.TextBlockParam{...},
                Tools:      []anthropic.ToolUnionParam{...},
                ToolChoice: anthropic.ToolChoiceParamOfTool("submit_evaluation"),
            },
        },
    },
}

// Batch status
anthropic.MessageBatchProcessingStatusInProgress  // "in_progress"
anthropic.MessageBatchProcessingStatusCanceling   // "canceling"
anthropic.MessageBatchProcessingStatusEnded       // "ended"

// Batch metadata
anthropic.MessageBatch{
    ID:               "batch_abc123",
    ProcessingStatus: "in_progress",
    RequestCounts: anthropic.MessageBatchRequestCounts{
        Processing: 10,
        Succeeded:  5,
        Errored:    0,
        Canceled:   0,
        Expired:    0,
    },
}

// Streaming results
stream := client.Messages.Batches.ResultsStreaming(ctx, batchID)
for stream.Next() {
    item := stream.Current()
    // item.CustomID  — matches our request
    // item.Result.Type — "succeeded", "errored", "canceled", "expired"
}

// Extracting tool_use from a succeeded result
succeeded := item.Result.AsSucceeded()
message := succeeded.Message  // same Message type as a normal API call
for i := range message.Content {
    if variant, ok := message.Content[i].AsAny().(anthropic.ToolUseBlock); ok {
        raw := json.RawMessage(variant.JSON.Input.Raw())
        // this is the same JSON we get from individual calls
    }
}
```

## Internal Architecture

```
caller 1 ──► Judge() ──► ch1 = make(chan result, 1) ──► batcher.submit(req, ch1) ──► <-ch1 ──► return
caller 2 ──► Judge() ──► ch2 = make(chan result, 1) ──► batcher.submit(req, ch2) ──► <-ch2 ──► return
caller 3 ──► Judge() ──► ch3 = make(chan result, 1) ──► batcher.submit(req, ch3) ──► <-ch3 ──► return
                                                              │
                                                              ▼
                                                     ┌─────────────────┐
                                                     │    batcher      │
                                                     │                 │
                                                     │  pending = [    │
                                                     │    {id, req, ch1}│
                                                     │    {id, req, ch2}│
                                                     │    {id, req, ch3}│
                                                     │  ]              │
                                                     │                 │
                                                     │  trigger: size  │
                                                     │  OR timer fires │
                                                     └────────┬────────┘
                                                              │
                                                              ▼ flush()
                                                     ┌─────────────────────────┐
                                                     │ Batches API             │
                                                     │                        │
                                                     │ client.Messages.       │
                                                     │   Batches.New(params)   │
                                                     │                        │
                                                     │ poll via .Get(batchID) │
                                                     │ until status == "ended"│
                                                     │                        │
                                                     │ stream results via     │
                                                     │   .ResultsStreaming()   │
                                                     └────────┬──────────────┘
                                                              │
                                                              ▼
                                                     fan out by custom_id:
                                                     ch1 <- result1
                                                     ch2 <- result2
                                                     ch3 <- result3
```

## Core Types

```go
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
    id   string          // custom_id for the batch API, matches result back to caller
    req  *callRequest    // the prompt, tool schema, model, etc.
    resp chan batchResult // buffered channel, capacity 1
}

// batcher collects requests and flushes them as a single batch API call.
type batcher struct {
    config  BatchConfig
    client  anthropic.Client

    once   sync.Once            // start background goroutine once
    inbox  chan pendingRequest   // callers submit here
    stop   chan struct{}         // signals shutdown

    idCounter atomic.Int64      // generates unique custom_ids
}
```

## Batcher Lifecycle

### Creation

```go
func newBatcher(cfg BatchConfig, apiKey string) *batcher {
    return &batcher{
        config: cfg,
        client: anthropic.NewClient(option.WithAPIKey(apiKey)),
        inbox:  make(chan pendingRequest),
        stop:   make(chan struct{}),
    }
}
```

### Background Goroutine

Started lazily on first submit via `sync.Once`. The `pending` slice and timer live entirely inside this goroutine — no mutex needed.

```go
func (b *batcher) run() {
    var pending []pendingRequest
    timer := time.NewTimer(b.config.MaxWait)
    timer.Stop()

    for {
        select {
        case req := <-b.inbox:
            pending = append(pending, req)

            // Start timer on first request in a new batch
            if len(pending) == 1 {
                timer.Reset(b.config.MaxWait)
            }

            // Flush on size threshold
            if len(pending) >= b.config.MaxSize {
                timer.Stop()
                b.flush(pending)
                pending = nil
            }

        case <-timer.C:
            if len(pending) > 0 {
                b.flush(pending)
                pending = nil
            }

        case <-b.stop:
            timer.Stop()
            if len(pending) > 0 {
                b.flush(pending)
            }
            return
        }
    }
}
```

### Submit (called by each goroutine)

Each caller gets a buffered channel (cap 1). Submit sends to the batcher's inbox (serialized by channel), then blocks on its own channel until the result arrives.

```go
func (b *batcher) submitRequest(req *callRequest) (json.RawMessage, *Usage, error) {
    b.once.Do(func() {
        go b.run()
    })

    ch := make(chan batchResult, 1)
    id := fmt.Sprintf("req-%d", b.idCounter.Add(1))

    b.inbox <- pendingRequest{
        id:   id,
        req:  req,
        resp: ch,
    }

    result := <-ch
    return result.raw, result.usage, result.err
}
```

### Flush

Converts pending requests to `MessageBatchNewParams`, submits via the SDK, polls until done, streams results, fans out by `custom_id`.

```go
func (b *batcher) flush(pending []pendingRequest) {
    ctx := context.Background()

    // Build batch request from pending items
    batchRequests := make([]anthropic.MessageBatchNewParamsRequest, len(pending))
    for i, p := range pending {
        batchRequests[i] = anthropic.MessageBatchNewParamsRequest{
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

    // Submit batch
    batch, err := b.client.Messages.Batches.New(ctx, anthropic.MessageBatchNewParams{
        Requests: batchRequests,
    })
    if err != nil {
        b.fanOutError(pending, fmt.Errorf("batch submit failed: %w", err))
        return
    }

    // Poll until complete
    batch, err = b.pollUntilDone(ctx, batch.ID)
    if err != nil {
        b.fanOutError(pending, fmt.Errorf("batch poll failed: %w", err))
        return
    }

    // Stream results and fan out
    b.fanOutResults(ctx, batch.ID, pending)
}

func (b *batcher) fanOutError(pending []pendingRequest, err error) {
    for _, p := range pending {
        p.resp <- batchResult{err: err}
    }
}

func (b *batcher) fanOutResults(ctx context.Context, batchID string, pending []pendingRequest) {
    // Build lookup map: custom_id → channel
    lookup := make(map[string]chan batchResult, len(pending))
    for _, p := range pending {
        lookup[p.id] = p.resp
    }

    // Stream results
    stream := b.client.Messages.Batches.ResultsStreaming(ctx, batchID)
    for stream.Next() {
        item := stream.Current()
        ch, ok := lookup[item.CustomID]
        if !ok {
            continue
        }

        switch item.Result.Type {
        case "succeeded":
            msg := item.Result.AsSucceeded().Message
            raw, usage := extractToolUse(msg)
            ch <- batchResult{raw: raw, usage: usage}

        case "errored":
            errored := item.Result.AsErrored()
            ch <- batchResult{err: fmt.Errorf("batch request failed: %v", errored.Error)}

        case "canceled":
            ch <- batchResult{err: errors.New("batch request was canceled")}

        case "expired":
            ch <- batchResult{err: errors.New("batch request expired")}
        }

        delete(lookup, item.CustomID)
    }

    if err := stream.Err(); err != nil {
        // Send error to any callers that didn't get a result
        for _, ch := range lookup {
            ch <- batchResult{err: fmt.Errorf("result stream failed: %w", err)}
        }
        return
    }

    // Any remaining callers didn't get a result
    for id, ch := range lookup {
        ch <- batchResult{err: fmt.Errorf("no result for request %s", id)}
    }
}

func extractToolUse(msg anthropic.Message) (json.RawMessage, *Usage) {
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
```

### Polling

Exponential backoff from 1s to 30s, respects context cancellation.

```go
func (b *batcher) pollUntilDone(ctx context.Context, batchID string) (*anthropic.MessageBatch, error) {
    interval := 1 * time.Second
    maxInterval := 30 * time.Second

    for {
        batch, err := b.client.Messages.Batches.Get(ctx, batchID)
        if err != nil {
            return nil, err
        }

        switch batch.ProcessingStatus {
        case anthropic.MessageBatchProcessingStatusEnded:
            return batch, nil
        case anthropic.MessageBatchProcessingStatusCanceling:
            // keep polling — it'll end eventually
        }

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
```

### Shutdown

```go
func (b *batcher) close() {
    close(b.stop)
}
```

## Integration with Existing Client

The `caller` interface stays the same. The batcher wraps it:

```go
type batchCaller struct {
    batcher *batcher
}

func (c *batchCaller) call(ctx context.Context, req *callRequest) (json.RawMessage, *Usage, error) {
    return c.batcher.submitRequest(req)
}
```

In `getClient()`:

```go
func getClient() caller {
    clientOnce.Do(func() {
        cfg := getConfig()
        if cfg.Batch != nil {
            globalClient = &batchCaller{
                batcher: newBatcher(*cfg.Batch, getAPIKey()),
            }
        } else {
            globalClient = newClaudeClient(getAPIKey())
        }
    })
    return globalClient
}
```

## Concurrency Details

### Thread Safety

| Component | Mechanism | Why |
|-----------|-----------|-----|
| `batcher.inbox` | Unbuffered channel | Multiple goroutines submit; channel serializes delivery to `run()` |
| `pending` slice | Single owner (`run` goroutine) | Never shared — no lock needed |
| Timer | Single owner (`run` goroutine) | Same |
| `batcher.once` | `sync.Once` | `run()` starts exactly once |
| `idCounter` | `atomic.Int64` | Lock-free unique ID generation across callers |
| Result fan-out | Buffered channel per caller (cap 1) | No contention — each caller owns its channel |
| `lookup` map in `fanOutResults` | Single goroutine | Built and consumed inside `flush`, which runs in `run()` |

### Why No Mutex on `pending`

The `pending` slice is only read and written inside the `run()` goroutine's select loop. Callers never touch it — they send to `inbox` and block on their own `resp` channel. This is the standard Go pattern: share memory by communicating, don't communicate by sharing memory.

### Timer Edge Cases

- Timer starts stopped — no spurious fires before first request
- Reset only on first request in a new batch (not every request)
- Stopped before flush on size trigger — prevents double flush
- If timer fires after a size-triggered flush empties pending: the `len(pending) > 0` guard makes it a no-op

## For E2E Tests

Tests use `t.Parallel()` so requests arrive concurrently. The batcher collects them during the `MaxWait` window and fires one batch.

```go
func TestMain(m *testing.M) {
    sense.Configure(sense.Config{
        Batch: &sense.BatchConfig{
            MaxSize: 50,
            MaxWait: 2 * time.Second,
        },
    })
    os.Exit(m.Run())
}

func TestEval_HighScore(t *testing.T) {
    t.Parallel()
    result, err := sense.Eval("good output").
        Expect("is well written").
        Judge()
    // ...
}
```

## What to Build

1. `BatchConfig` type in `config.go`
2. `batcher` struct in `batch.go` — inbox channel, run goroutine, flush, poll, fan-out
3. `batchCaller` implementing `caller` in `batch.go`
4. Wire into `getClient()` — if `Batch` config is set, use `batchCaller`
5. Unit tests with mock batch client
6. E2E test with `t.Parallel()` + batch config

## Open Questions

1. **Batch processing time** — batches can take minutes. For 15 test requests this is likely fast (<30s), but we need a timeout. Default to 5 minutes?
2. **Error granularity** — per-request errors (one request fails, others succeed). Handled in `fanOutResults` by checking each result's type independently.
3. **Context cancellation** — if a caller's context is canceled while waiting on `<-ch`, the batcher still processes the request but the result is discarded (sent to a channel nobody reads, then GC'd). This is fine — no goroutine leak.
4. **Fallback** — should we fall back to individual calls if the batch is too small (e.g., 1 request)? A batch of 1 request has higher latency than a direct call due to polling. Could add a `MinSize` threshold.
5. **MaxWait tuning** — for e2e tests, 2s is probably right. For production use, 100-500ms. Make it configurable per the `BatchConfig`.
