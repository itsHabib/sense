# Sense — Feature Backlog

What's shipped and what's on deck. Features are grouped by category, not ordered — pick what matters most for the current use case.

---

## Shipped

### Cost Tracking

Atomic token counters across all operations. `s.Usage()` returns a `SessionUsage` snapshot. Done.

---

## Eval Quality

### Custom Evaluators (Deterministic Checks)

Run local checks before the LLM call. If any fail, skip the API call entirely — saves cost.

```go
s.Assert(t, output).
    Expect("tone is professional").         // LLM-judged
    Check(sense.ValidJSON()).               // built-in deterministic check
    Check(sense.MaxLength(5000)).           // built-in
    Check(sense.Matches(`"status":\s*"`)).  // regex
    Check(myCustomCheck).                   // user-defined
    Run()
```

User-defined evaluator:

```go
var myCustomCheck = sense.CheckFunc("has required headers", func(output string) sense.CheckResult {
    missing := []string{}
    for _, h := range []string{"Title", "Summary", "Conclusion"} {
        if !strings.Contains(output, h) {
            missing = append(missing, h)
        }
    }
    if len(missing) > 0 {
        return sense.CheckResult{
            Pass:   false,
            Reason: fmt.Sprintf("missing sections: %s", strings.Join(missing, ", ")),
        }
    }
    return sense.CheckResult{Pass: true, Reason: "all required sections present"}
})
```

**What to build:**

- `Checker` interface: `Check(output string) CheckResult`
- `CheckResult` struct: `Pass`, `Reason`, `Confidence` (always 1.0 for deterministic)
- `CheckFunc` helper to create a `Checker` from a function
- Built-in checkers: `ValidJSON()`, `ValidYAML()`, `MaxLength(n)`, `MinLength(n)`, `Matches(regex)`, `Contains(substr)`, `NotContains(substr)`
- `Check()` method on `AssertBuilder` and `EvalBuilder`
- Deterministic checks run first (fast, free). If any fail, skip the LLM call entirely
- Results merge into the same `EvalResult.Checks` slice with `Confidence: 1.0`

### Extract Validation

Run validation on the extracted struct *after* extraction. Unlike Eval checks (which operate on raw text and run *before* the LLM call), Extract checks operate on the typed result and run *after*.

```go
result, err := sense.Extract[Order](rawEmail).
    Validate(func(o Order) error {
        if o.Total < 0 {
            return errors.New("negative total")
        }
        if o.Items == 0 {
            return errors.New("order has no items")
        }
        return nil
    }).
    Run()
```

**What to build:**

- `Validate(func(T) error)` method on `ExtractBuilder[T]`
- Runs after successful extraction, before returning result
- Validation errors wrap as `&Error{Op: "extract", Message: "validation failed", Err: err}`
- Multiple `Validate()` calls chain — all run, first error wins
- No LLM cost — validation is pure Go on the typed struct

### Snapshots

Save eval results to disk. On subsequent runs, compare against the snapshot. Catch regressions. This is what turns Sense from "assertion library" into "eval framework."

```go
// First run: saves to .snapshots/TestMyAgent.json
// Later runs: compares against saved snapshot, flags regressions
s.Assert(t, output).
    Expect("produces valid Go code").
    Expect("handles errors idiomatically").
    Snapshot().
    Run()
```

Update snapshots explicitly:

```bash
SENSE_UPDATE_SNAPSHOTS=1 go test ./...
```

**What to build:**

- `Snapshot()` method on `AssertBuilder` and `EvalBuilder`
- Snapshot file: JSON with test name, expectations, results, scores, timestamp
- On run: load previous snapshot, compare scores/pass-fail per expectation
- Regression = a check that previously passed now fails, or score dropped > threshold
- `SENSE_UPDATE_SNAPSHOTS=1` overwrites the snapshot file with current results
- Snapshot dir defaults to `.snapshots/` relative to test file, configurable via `Config`

### Multi-Judge Consensus

Run the same evaluation against multiple models. Require agreement for a pass.

```go
s := sense.New(sense.WithConsensus(
    sense.ConsensusAll,
    "claude-sonnet-4-6", "claude-haiku-4-5-20251001",
))

result, err := s.Eval(output).
    Expect("is factually accurate").
    Judge()

for _, j := range result.Judgments {
    fmt.Printf("%s: pass=%v score=%.2f\n", j.Model, j.Pass, j.Score)
}
```

**Strategies:**

| Strategy | Behavior |
|----------|----------|
| `ConsensusAll` | All models must pass every check |
| `ConsensusMajority` | Majority of models must pass each check |
| `ConsensusAny` | At least one model must pass each check |

**What to build:**

- `WithConsensus` functional option — strategy + models list
- When consensus is configured, `JudgeContext` fans out to N models concurrently
- Each model gets the same prompt, returns its own `EvalResult`
- Merge results based on strategy: aggregate pass/fail, average scores, combine reasons
- `EvalResult` gets a `Judgments []ModelJudgment` field for per-model breakdown
- Works with both individual calls and batching

---

## Dataset & Scale

### Dataset Runner

Run the same eval across N inputs from a file and aggregate results. Without this, you're testing 5 handpicked examples and hoping they generalize.

```go
sense.Dataset("testdata/summarization.jsonl").
    ForEach(func(d sense.DataPoint) {
        s.Assert(t, myLLM(d.Input)).
            Expect(d.Expected).Run()
    }).
    Report()  // "87/100 passed, 3 regressions from last run"
```

**What to build:**

- `Dataset(path)` loader — supports JSONL, CSV, JSON array
- `DataPoint` struct: `Input`, `Expected`, `Metadata map[string]string`
- `ForEach` runs the eval function against every data point
- `Report()` aggregates: total, passed, failed, pass rate, regressions from last snapshot
- Integrates with snapshots — dataset results are snapshot-able as a whole
- Configurable concurrency for parallel execution across data points

### Parallel Eval Execution

Run N evals concurrently with rate limiting. Users shouldn't have to manage goroutines + semaphores themselves.

```go
s := sense.New(sense.WithConcurrency(10))  // max 10 concurrent API calls
```

**What to build:**

- `WithConcurrency(n)` option — semaphore on the caller layer
- Rate-limit aware — back off on 429s globally, not per-goroutine
- Works transparently with Dataset runner
- Default: sequential (concurrency = 1) to preserve current behavior

### ExtractSlice[T]

Extract a list of items from one text. Invoices have line items. Logs have multiple events. Emails have multiple entities.

```go
type LineItem struct {
    Product  string  `json:"product"`
    Quantity int     `json:"quantity"`
    Price    float64 `json:"price"`
}

results, err := sense.ExtractSlice[LineItem](s, invoiceText).Run()
for _, item := range results.Data {
    fmt.Printf("%s x%d @ $%.2f\n", item.Product, item.Quantity, item.Price)
}
```

**What to build:**

- `ExtractSlice[T]()` function that returns `*ExtractSliceBuilder[T]`
- `ExtractSliceResult[T]` with `Data []T`
- Schema: wrap T in `{"type": "object", "properties": {"items": {"type": "array", "items": <schema for T>}}}`
- Unwrap the `items` array after extraction
- Same builder methods: `Context()`, `Model()`, `Validate()`, `Run()`, `RunContext()`
- Validate runs on each item individually

---

## Cost & Safety

### Cost Budget

Prevent runaway costs. One bad loop or large dataset run can burn $50+ before you notice. Teams stop running evals when they can't predict cost.

```go
s := sense.New(sense.WithMaxCost(sense.Dollars(0.50)))
```

**What to build:**

- `WithMaxCost` functional option (float64, in dollars)
- `Dollars(n float64) float64` helper for readability
- After each `recordUsage` call, estimate cost from token counts using model pricing
- If accumulated cost exceeds `MaxCost`, return `ErrBudgetExceeded` on subsequent calls
- Model pricing: hardcoded table, updatable. Start with Sonnet/Haiku/Opus pricing
- `SessionUsage` gains an `EstimatedCost` field

### FileCache

Persist responses across runs. During iterative development, you re-evaluate the exact same prompt dozens of times. FileCache eliminates redundant API calls entirely.

```go
s := sense.New(sense.WithCache(sense.FileCache(".sense-cache")))
```

**What to build:**

- Implement `FileCache.Get` and `FileCache.Set` (currently no-op stubs)
- Cache key: SHA-256 of `(system_prompt, user_message, tool_schema, model)`
- Storage: one file per cache entry, key as filename, response bytes as content
- Wire cache into the `caller` layer — check cache before API call, store after
- No TTL by default (LLM responses to identical inputs are stable). Optional `MaxAge` for staleness control

### Anthropic Prompt Caching

Use Anthropic's `cache_control` to reduce cost on repeated system prompts within a session. Transparent optimization — no API surface change.

**What to build:**

- Add `cache_control: {"type": "ephemeral"}` to the system message in `callRequest`
- First call pays full price. Subsequent calls with the same system prompt pay 90% less for those tokens
- No user-facing API change

---

## CI & Integration

### JUnit XML Reporter

Standard CI output format that every CI system can ingest.

```go
s := sense.New(sense.WithReporter(sense.NewJUnitReporter("sense-results.xml")))
defer s.Close() // flushes report on close
```

**What to build:**

- `Reporter` interface: `Record(testName string, result EvalResult)`, `Flush() error`
- `JUnitReporter` implementation: accumulates results, writes JUnit XML on `Flush()`
- Each eval maps to a JUnit test case. Failed checks become failure elements with reason + evidence
- Session calls `Reporter.Record()` after every eval/assert
- `Close()` calls `Reporter.Flush()` automatically

### GitHub Actions Annotations

Zero-config. Detect `GITHUB_ACTIONS=true` and emit annotations inline on PRs.

**What to build:**

- `GitHubReporter` implementation of `Reporter`
- Emits `::error file=...,line=...::` and `::warning file=...,line=...::` to stdout
- Failed checks -> error annotations. Low-confidence passes -> warning annotations
- Auto-enabled when `GITHUB_ACTIONS=true` unless explicitly disabled

### Multi-Model Judges

Sense only works with Claude today. The `Caller` interface is already abstracted — adding an OpenAI implementation is ~100 lines. This unlocks:

- Judge outputs from any model (already possible — it's just text input)
- Judge *with* different models to cross-validate
- Use Sense without an Anthropic API key

**What to build:**

- `OpenAICaller` implementing the `Caller` interface
- `WithCaller(c Caller)` option to inject a custom caller
- Same tool_use pattern via OpenAI's function calling
- Enables multi-model consensus with heterogeneous judges

---

## API Surface

### Session Interface

Session is a struct. Users who embed Sense in production code can't mock it without the real API.

```go
type Evaluator interface {
    Assert(t testing.TB, output any) *AssertBuilder
    Require(t testing.TB, output any) *AssertBuilder
    Eval(output any) *EvalBuilder
    Compare(a, b any) *CompareBuilder
    Usage() SessionUsage
    Close()
}
```

**What to build:**

- `Evaluator` interface matching the public methods on `Session`
- `Session` already satisfies it — no code changes needed
- Export in sense.go alongside the struct

---

## Positioning: Extract as co-lead

Not a code change — a narrative change. The README currently positions Sense as a testing tool that also does extraction. Extract is independently valuable for any Go program that processes unstructured text:

| Use case | Example |
|----------|---------|
| Log parsing | Parse unstructured app logs into typed events |
| Support tickets | Classify and extract fields from customer emails |
| Document processing | Pull structured data from contracts, invoices, reports |
| API normalization | Normalize inconsistent third-party API responses |
| Migration tooling | Parse legacy config formats into new structs |
| Monitoring | Extract error patterns from alert text |

The README should lead with two tracks: **"Judge output"** (Assert/Eval/Compare) and **"Extract structure"** (Extract). Not "testing framework that also does extraction."
