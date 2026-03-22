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
        MaxSize:   20,                    // flush after 20 requests
        MaxWait:   500 * time.Millisecond, // or flush after 500ms
    },
})
```

When `Batch` is nil (default), requests go directly to Claude as individual calls — current behavior, no change.

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
                                                     │  mu.Lock()      │
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
                                                     ┌─────────────────┐
                                                     │ Batches API     │
                                                     │                 │
                                                     │ POST /batches   │
                                                     │ {requests: [...]}│
                                                     │                 │
                                                     │ poll until done │
                                                     └────────┬────────┘
                                                              │
                                                              ▼
                                                     fan out results:
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
    mu      sync.Mutex
    pending []pendingRequest
    timer   *time.Timer
    config  BatchConfig
    client  *anthropic.Client // raw client for batch API calls

    once    sync.Once   // start the background goroutine once
    submit  chan pendingRequest
    stop    chan struct{}
}
```

## Batcher Lifecycle

### Creation

```go
func newBatcher(cfg BatchConfig, apiKey string) *batcher {
    return &batcher{
        config: cfg,
        client: anthropic.NewClient(option.WithAPIKey(apiKey)),
        submit: make(chan pendingRequest),
        stop:   make(chan struct{}),
    }
}
```

### Background Goroutine

Started lazily on first `submit()` call via `sync.Once`.

```go
func (b *batcher) run() {
    var pending []pendingRequest
    timer := time.NewTimer(b.config.MaxWait)
    timer.Stop() // don't fire until we have requests

    for {
        select {
        case req := <-b.submit:
            pending = append(pending, req)

            // Start/reset timer on first request in a batch
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
            // Flush on time threshold
            if len(pending) > 0 {
                b.flush(pending)
                pending = nil
            }

        case <-b.stop:
            // Flush remaining on shutdown
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

```go
func (b *batcher) submitRequest(req *callRequest) (json.RawMessage, *Usage, error) {
    // Start background goroutine on first call
    b.once.Do(func() {
        go b.run()
    })

    ch := make(chan batchResult, 1)
    id := uuid() // or sequential counter with mutex

    b.submit <- pendingRequest{
        id:   id,
        req:  req,
        resp: ch,
    }

    // Block until our result arrives
    result := <-ch
    return result.raw, result.usage, result.err
}
```

### Flush (submit batch, poll, fan out)

```go
func (b *batcher) flush(pending []pendingRequest) {
    // Build batch request — each pending request becomes a batch item
    // with its custom_id set to the pending request's id
    batchReq := buildBatchRequest(pending)

    // Submit to Anthropic Batches API
    batch, err := b.client.Batches.Create(ctx, batchReq)
    if err != nil {
        // Send error to all waiting callers
        for _, p := range pending {
            p.resp <- batchResult{err: err}
        }
        return
    }

    // Poll until batch completes
    for batch.Status != "ended" {
        time.Sleep(pollInterval) // start at 1s, back off
        batch, err = b.client.Batches.Get(ctx, batch.ID)
        if err != nil {
            for _, p := range pending {
                p.resp <- batchResult{err: err}
            }
            return
        }
    }

    // Retrieve results and fan out to callers
    results := b.client.Batches.Results(ctx, batch.ID)
    resultMap := indexByCustomID(results)

    for _, p := range pending {
        r, ok := resultMap[p.id]
        if !ok {
            p.resp <- batchResult{err: errors.New("no result for request")}
            continue
        }
        raw, usage := extractToolResult(r)
        p.resp <- batchResult{raw: raw, usage: usage}
    }
}
```

## Integration with Existing Client

The `caller` interface stays the same. The batcher wraps it:

```go
// batchCaller implements caller by routing through the batcher.
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
| `batcher.submit` channel | Channel send | Multiple goroutines submit concurrently, channel serializes |
| `pending` slice | Single goroutine (`run`) | Only accessed inside the `run` goroutine — no lock needed |
| `timer` | Single goroutine (`run`) | Same — only `run` touches it |
| `batcher.once` | `sync.Once` | Ensures `run()` goroutine starts exactly once |
| ID generation | `sync.Mutex` + counter, or `atomic.Int64` | Unique IDs across concurrent callers |
| Result fan-out | Buffered channels (cap 1) | Each caller has its own channel, no contention |

### No Mutex on `pending`

The `pending` slice lives entirely inside the `run()` goroutine. Callers don't touch it — they send to the `submit` channel and block on their `resp` channel. The `run` loop is the only reader/writer of `pending`, so no mutex is needed.

### Timer Management

- Timer is created stopped
- Reset on first request in a new batch
- Stopped when flush happens (size trigger or shutdown)
- If timer fires and pending is empty (race between size flush and timer), no-op

### Shutdown

```go
func (b *batcher) close() {
    close(b.stop) // signals run() to flush remaining and exit
}
```

Call in `TestMain` cleanup or `Configure` reset.

## Polling Strategy

The Batches API is async — we submit and poll. Polling strategy:

```go
func (b *batcher) pollUntilDone(ctx context.Context, batchID string) (*BatchResult, error) {
    interval := 1 * time.Second
    maxInterval := 30 * time.Second

    for {
        batch, err := b.client.Batches.Get(ctx, batchID)
        if err != nil {
            return nil, err
        }

        switch batch.Status {
        case "ended":
            return batch, nil
        case "errored", "expired", "canceled":
            return nil, fmt.Errorf("batch %s: %s", batch.Status, batchID)
        }

        // Exponential backoff on polling
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(interval):
        }

        interval = min(interval*2, maxInterval)
    }
}
```

## For E2E Tests

Tests use `t.Parallel()` so requests arrive concurrently. The batcher collects them during the `MaxWait` window and fires one batch.

```go
func TestMain(m *testing.M) {
    sense.Configure(sense.Config{
        Batch: &sense.BatchConfig{
            MaxSize: 50,
            MaxWait: 2 * time.Second, // wait for all tests to register
        },
    })
    os.Exit(m.Run())
}

func TestEval_HighScore(t *testing.T) {
    t.Parallel() // allows batcher to collect multiple requests
    result, err := sense.Eval("good output").
        Expect("is well written").
        Judge()
    // ...
}
```

## What to Build

1. `BatchConfig` type in `config.go`
2. `batcher` struct in `batch.go` — submit channel, run goroutine, flush, poll
3. `batchCaller` implementing `caller` in `batch.go`
4. Wire into `getClient()` — if `Batch` config is set, use `batchCaller`
5. Tests with mock batcher

## Open Questions

1. **Does the Anthropic Go SDK expose the Batches API?** Need to check. If not, raw HTTP calls against `POST /v1/messages/batches`.
2. **Batch processing time** — if batches take 5+ minutes, the test suite blocks for that long. May need a timeout or fallback to individual calls for small batches.
3. **Error granularity** — if one request in a batch fails, do we fail just that caller or the whole batch? Per-request errors are better.
4. **Context cancellation** — if a caller's context is canceled while waiting, should we remove their request from the pending batch? Or let it run and discard the result?
5. **MaxWait tuning** — too short and we get many small batches (no benefit). Too long and tests feel slow waiting. 500ms-2s feels right for tests.
