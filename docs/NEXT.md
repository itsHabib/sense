# Sense — Roadmap

What's shipped, what's next, and why — in priority order.

---

## Shipped

### Cost Tracking (v0.1.0)

Atomic token counters across all operations. `s.Usage()` returns a `SessionUsage` snapshot. Done.

---

## v0.2: Custom Evaluators + Extract Validation

Merge two ideas: deterministic pre-LLM checks on Eval/Assert, and typed post-extraction validation on Extract.

### Deterministic Checks (Eval/Assert)

Run local checks before the LLM call. If any fail, skip the API call entirely — saves cost.

**API:**

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

**Why it matters:**

Not everything needs an LLM judge. "Is this valid JSON?" is a `json.Unmarshal` call, not a $0.003 API request. Mixing deterministic and semantic checks in one assertion gives you fast feedback on the obvious stuff and LLM judgment on the fuzzy stuff. Running deterministic checks first also lets you fail fast — no point asking Claude if the output "handles errors well" when it's not even valid Go.

### Extract Validation (typed checks on extracted structs)

Run validation on the extracted struct *after* extraction. Unlike Eval checks (which operate on raw text and run *before* the LLM call), Extract checks operate on the typed result and run *after*.

**API:**

```go
type Order struct {
    CustomerID string  `json:"customer_id" sense:"The customer identifier"`
    Total      float64 `json:"total" sense:"Order total in dollars"`
    Items      int     `json:"items" sense:"Number of line items"`
}

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

**Why it matters:**

Extract produces structure from chaos, but the LLM can hallucinate plausible-looking values. A negative order total, a date in the future, a missing required field — these are things a typed check catches instantly. Without this, every Extract call needs manual if-statements after it. With it, validation is part of the extraction pipeline.

---

## v0.3: FileCache + Prompt Caching

Two layers of cost reduction that stack.

### FileCache

Finish the stubbed `FileCache` implementation and wire caching into the call path.

**API:**

```go
s := sense.New(sense.WithCache(sense.FileCache(".sense-cache")))
```

**What to build:**

- Implement `FileCache.Get` and `FileCache.Set` (currently no-op stubs)
- Cache key: SHA-256 of `(system_prompt, user_message, tool_schema, model)`
- Storage: one file per cache entry, key as filename, response bytes as content
- Wire cache into the `caller` layer — check cache before API call, store after
- Cache is per-session, opt-in via `Config.Cache`
- No TTL by default (LLM responses to identical inputs are stable). Optional `MaxAge` on `FileCache` for staleness control

**Why it matters:**

During iterative development, you re-evaluate the *exact same prompt* dozens of times while tweaking tests. That's burning money for identical results. FileCache eliminates redundant API calls entirely. MemoryCache already exists but only helps within a single process. FileCache persists across runs.

### Anthropic Prompt Caching

Use Anthropic's `cache_control` to reduce cost on repeated system prompts within a session.

**What to build:**

- Add `cache_control: {"type": "ephemeral"}` to the system message in `callRequest`
- The three system prompts (`evalSystemPrompt`, `compareSystemPrompt`, `extractSystemPrompt`) are identical across every call of their type
- Anthropic caches the KV state server-side within a 5-minute TTL
- First call pays full price. Subsequent calls with the same system prompt pay 90% less for those tokens
- No API surface change — transparent optimization in the client layer

**Why it matters:**

A test suite that runs 20 evals in sequence sends the same ~500-token system prompt 20 times. With prompt caching, you pay full price once and 90% less on the remaining 19. Free money for anyone running more than a handful of evals per session. Stacks with FileCache — prompt caching helps on cache-miss calls, FileCache eliminates cache-hit calls entirely.

---

## v0.4: Snapshots + CI Reporter

Ship together — snapshots without CI integration is just JSON files nobody looks at.

### Snapshots

Save eval results to disk. On subsequent runs, compare against the snapshot. Catch regressions.

**API:**

```go
// First run: saves to .snapshots/TestMyAgent.json
// Later runs: compares against saved snapshot, flags regressions
s.Assert(t, output).
    Expect("produces valid Go code").
    Expect("handles errors idiomatically").
    Snapshot().  // enable snapshot mode
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

### JUnit XML Reporter

Standard CI output format that every CI system on earth can ingest.

**API:**

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
- Score and confidence go into JUnit properties for trend tracking

### GitHub Actions Annotations

Optional, zero-config. Detect `GITHUB_ACTIONS=true` and emit annotations.

**What to build:**

- `GitHubReporter` implementation of `Reporter`
- Emits `::error file=...,line=...::` and `::warning file=...,line=...::` to stdout
- Failed checks → error annotations. Low-confidence passes → warning annotations
- Auto-enabled when `GITHUB_ACTIONS=true` unless explicitly disabled
- Shows inline on PRs — reviewer sees exactly which eval checks failed

**Why it matters (all three):**

You change a system prompt, run your evals, everything passes. But did quality actually degrade? Without snapshots you'd never know that "handles errors idiomatically" went from 0.95 confidence to 0.60. Snapshots make regressions visible. JUnit XML makes them visible *in CI*. GitHub annotations make them visible *on the PR*. This is what turns sense from "a tool I run locally" to "part of our quality gate."

---

## v0.5: Multi-Judge Consensus

Run the same evaluation against multiple models. Require agreement for a pass. Ship this after caching exists (so 3x calls isn't 3x cost on cache-warm runs) and after CI integration (so consensus results show up in reports).

**API:**

```go
s := sense.New(sense.WithConsensus(
    sense.ConsensusAll,
    "claude-sonnet-4-6", "claude-haiku-4-5-20251001",
))

// Same API — consensus is transparent
result, err := s.Eval(output).
    Expect("is factually accurate").
    Judge()

// Result includes per-model breakdown
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

**Why it matters:**

A single model can have blind spots. Haiku might miss a subtle factual error that Sonnet catches. Running both and requiring agreement gives you higher confidence in your evals. The cost is 2-3x but the reliability gain is significant for critical assertions. With caching and prompt caching already in place, the cost multiplier is blunted.

---

## Slot into any release

These are smaller features that can ship alongside any version.

### ExtractSlice[T] — extract multiple items from one text

Right now Extract returns a single `T`. Real-world extraction often produces a list.

**API:**

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

**Why it matters:**

Invoices have line items. Logs have multiple events. Emails have multiple entities. Forcing users to wrap their struct in a slice-holder struct is friction that a first-class `ExtractSlice` eliminates.

### Session Interface — make sense mockable

Session is a struct. Users who embed sense in production code can't mock it for unit tests without hitting the real API or using `SENSE_SKIP=1` (which tests nothing).

**API:**

```go
// Evaluator is the interface satisfied by Session.
// Use this in function signatures to enable mocking.
type Evaluator interface {
    Assert(t testing.TB, output any) *AssertBuilder
    Require(t testing.TB, output any) *AssertBuilder
    Eval(output any) *EvalBuilder
    Compare(a, b any) *CompareBuilder
    Usage() SessionUsage
    Close()
}

// In user code:
func ProcessDocument(e sense.Evaluator, doc string) error {
    result, err := e.Eval(doc).Expect("is complete").Judge()
    // ...
}

// In user tests:
type mockSense struct { /* ... */ }
func (m *mockSense) Eval(output any) *sense.EvalBuilder { /* return canned */ }
```

**What to build:**

- `Evaluator` interface matching the public methods on `Session`
- `Session` already satisfies it — no code changes needed
- Export in sense.go alongside the struct
- Consider: does `Extract[T]()` being a package-level generic function (not a method) create friction here? May need `Extractor[T]` interface or accept that Extract mocking requires a different pattern

**Why it matters:**

Adoption blocker for teams. If my service uses sense in production (not just tests), I need to mock it in my unit tests. Right now I can't without the real API key. An interface costs nothing to add and unblocks every team that uses sense beyond `_test.go` files.

### Cost Budget — fail if a session exceeds a threshold

Prevent runaway costs, especially important with consensus (where misconfigured fan-out could 10x a bill).

**API:**

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

**Why it matters:**

In CI, a misconfigured test suite or a consensus fan-out can burn through API credits fast. A hard cap turns a $50 surprise into a $0.50 controlled failure. Simple to implement — one check after `recordUsage`, one pricing lookup table.

---

## Positioning: Extract as co-lead

Not a code change — a narrative change. The README and package doc currently position sense as a testing tool that also does extraction. Extract is independently valuable for any Go program that processes unstructured text:

| Use case | Example |
|----------|---------|
| Log parsing | Parse unstructured app logs into typed events |
| Support tickets | Classify and extract fields from customer emails |
| Document processing | Pull structured data from contracts, invoices, reports |
| API normalization | Normalize inconsistent third-party API responses |
| Migration tooling | Parse legacy config formats into new structs |
| Monitoring | Extract error patterns from alert text |

The forced `tool_use` approach is the differentiator — no prompt engineering, no JSON parsing, no "hope the model returns valid output." Struct tags as the schema definition is idiomatic Go.

The README should lead with two tracks: **"Judge output"** (Assert/Eval/Compare) and **"Extract structure"** (Extract). Not "testing framework that also does extraction."

---

## Build order summary

| Version | Feature | Impact |
|---------|---------|--------|
| v0.1.0 | Cost Tracking | **Shipped** |
| v0.2 | Custom Evaluators + Extract Validation | Highest immediate value. Enables validation pipelines. Saves API cost via fail-fast |
| v0.3 | FileCache + Prompt Caching | Cut costs 50-90% during dev. Finish the stubbed FileCache |
| v0.4 | Snapshots + JUnit Reporter + GH Annotations | Unlock CI adoption. Snapshots + reporting = quality gate |
| v0.5 | Multi-Judge Consensus | Power feature. Ship when there's pull, after caching blunts the cost |
| Any | ExtractSlice[T] | Natural extension, real demand, small scope |
| Any | Session Interface | Small change, big adoption unblock |
| Any | Cost Budget | Safety net, especially for consensus |
