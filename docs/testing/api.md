# API Reference

Package: `github.com/itsHabib/agentkit`

---

## Configuration

### func Configure

```go
func Configure(cfg Config)
```

Sets global defaults. Call in `TestMain` or `init`. Everything has sane defaults — you only need to set `APIKey` if `ANTHROPIC_API_KEY` isn't in your environment.

```go
func TestMain(m *testing.M) {
    agent.Configure(agent.Config{
        Cache: agent.FileCache("testdata/agent-cache"),
    })
    os.Exit(m.Run())
}
```

### type Config

```go
type Config struct {
    // APIKey for Claude. Default: $ANTHROPIC_API_KEY
    APIKey string

    // Model for evaluations. Default: "claude-sonnet-4-6"
    Model string

    // Cache for response caching. Default: nil (no caching)
    Cache Cache

    // Timeout per API call. Default: 30s
    Timeout time.Duration

    // MaxRetries on transient failures. Default: 3
    MaxRetries int
}
```

---

## Core API

### func Assert

```go
func Assert(t testing.TB, output any) *AssertBuilder
```

Creates a test assertion. Calls `t.Fatal()` if any expectation fails. The output can be a string, `[]byte`, or any type (structs are serialized to JSON).

```go
agent.Assert(t, doc).
    Expect("covers the requirements").
    Expect("includes code examples").
    Run()
```

### func Eval

```go
func Eval(output any) *EvalBuilder
```

Evaluates output without failing a test. Returns a result you can inspect programmatically.

```go
result, err := agent.Eval(doc).
    Expect("is well-structured").
    Expect("has actionable items").
    Judge()

fmt.Println(result.Score)  // 0.85
for _, c := range result.FailedChecks() {
    fmt.Println(c.Expect, "—", c.Reason)
}
```

### func Compare

```go
func Compare(a, b any) *CompareBuilder
```

A/B comparison of two outputs.

```go
cmp, err := agent.Compare(docV1, docV2).
    Criteria("completeness").
    Criteria("clarity").
    Judge()

fmt.Println(cmp.Winner) // "B"
```

---

## AssertBuilder

```go
// Expect adds a natural language expectation. Chainable.
func (b *AssertBuilder) Expect(expectation string) *AssertBuilder

// Context adds background information for the judge.
func (b *AssertBuilder) Context(ctx string) *AssertBuilder

// Quick uses regex/heuristics instead of an API call. Free and instant.
func (b *AssertBuilder) Quick() *AssertBuilder

// Consensus runs N independent judges, passes on majority agreement.
func (b *AssertBuilder) Consensus(n int) *AssertBuilder

// Model overrides the judge model for this assertion.
func (b *AssertBuilder) Model(model string) *AssertBuilder

// Run executes the assertion. Calls t.Fatal() on failure.
// No return value — it either passes silently or kills the test.
func (b *AssertBuilder) Run()
```

---

## EvalBuilder

```go
// Expect adds a natural language expectation. Chainable.
func (b *EvalBuilder) Expect(expectation string) *EvalBuilder

// Context adds background information for the judge.
func (b *EvalBuilder) Context(ctx string) *EvalBuilder

// Quick uses regex/heuristics instead of an API call.
func (b *EvalBuilder) Quick() *EvalBuilder

// Consensus runs N independent judges.
func (b *EvalBuilder) Consensus(n int) *EvalBuilder

// Model overrides the judge model.
func (b *EvalBuilder) Model(model string) *EvalBuilder

// Judge executes the evaluation and returns the result.
func (b *EvalBuilder) Judge() (*EvalResult, error)
```

---

## CompareBuilder

```go
// Criteria adds a comparison dimension. Chainable.
func (b *CompareBuilder) Criteria(criterion string) *CompareBuilder

// Context adds background information.
func (b *CompareBuilder) Context(ctx string) *CompareBuilder

// Model overrides the judge model.
func (b *CompareBuilder) Model(model string) *CompareBuilder

// Judge executes the comparison.
func (b *CompareBuilder) Judge() (*CompareResult, error)
```

---

## Result Types

### EvalResult

```go
type EvalResult struct {
    Pass         bool          // All checks passed
    Score        float64       // 0.0 – 1.0 aggregate score
    Checks       []Check       // Per-expectation results
    Duration     time.Duration // Wall clock time
    TokensUsed   int           // Total tokens consumed
    Model        string        // Model used
    CacheHit     bool          // Served from cache
}

func (r *EvalResult) FailedChecks() []Check
func (r *EvalResult) PassedChecks() []Check
```

### Check

```go
type Check struct {
    Expect     string  // The expectation text
    Pass       bool    // Did it pass
    Confidence float64 // 0.0 – 1.0
    Reason     string  // Why it passed or failed
    Evidence   string  // Specific quotes from the output
}
```

### CompareResult

```go
type CompareResult struct {
    Winner   string            // "A", "B", or "tie"
    ScoreA   float64           // 0.0 – 1.0
    ScoreB   float64           // 0.0 – 1.0
    Criteria []CriterionResult // Per-criterion breakdown
}

type CriterionResult struct {
    Name   string  // Criterion name
    ScoreA float64
    ScoreB float64
    Winner string  // "A", "B", or "tie"
    Reason string
}
```

---

## Cache

```go
// FileCache creates a disk-based cache. Responses stored as JSON files
// named by content hash. Commit the directory to your repo for free CI.
func FileCache(dir string) Cache

// MemoryCache creates an in-memory cache. Useful for test isolation.
func MemoryCache() Cache
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | Claude API key | Required (or set in Config) |
| `AGENTKIT_MODEL` | Default judge model | `claude-sonnet-4-6` |
| `AGENTKIT_TIMEOUT` | Default timeout | `30s` |
| `AGENTKIT_SKIP` | Set to `1` to skip all agent assertions (for offline dev) | unset |

`AGENTKIT_SKIP` is useful for offline development — all `Assert` and `Eval` calls become no-ops, so `go test` still runs without an API key.
