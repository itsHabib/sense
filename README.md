# Sense

Make sense of non-deterministic output. Extract structured data from text and evaluate output quality using Claude.

```go
// Judge: output → pass/fail with evidence
sense.Assert(t, output).
    Expect("covers all sections from the brief").
    Expect("includes actionable recommendations").
    Run()

// Extract: unstructured text → typed struct
s := sense.New()
var m MountError
s.Extract("device /dev/sdf already mounted with vol-0abc123", &m).Run()
fmt.Println(m.Device)   // "/dev/sdf"
fmt.Println(m.VolumeID) // "vol-0abc123"
```

Sense uses the [Anthropic API](https://docs.anthropic.com/en/docs) (Claude) with forced `tool_use` for structured responses — no prompt engineering, no JSON parsing on your end. Requires an Anthropic API key.

- **Extract** — parse unstructured text into typed Go structs. Logs, error messages, support tickets, API responses — define a struct, get structured data back.
- **Judge** — evaluate non-deterministic output against expectations. Assert in tests, eval programmatically, or A/B compare two outputs.

## Install

```bash
go get github.com/itsHabib/sense
```

```bash
export ANTHROPIC_API_KEY=...
```

## Extract — structure from chaos

Define a struct. Get structured data back. Works with any text.

```go
type MountError struct {
    Device   string `json:"device" sense:"The device path"`
    VolumeID string `json:"volume_id" sense:"The EBS volume ID"`
    Message  string `json:"message"`
}

s := sense.New()

var m MountError
_, err := s.Extract("device /dev/sdf already mounted with vol-0abc123", &m).
    Context("AWS EC2 EBS error messages").
    Run()

fmt.Println(m.Device)   // "/dev/sdf"
fmt.Println(m.VolumeID) // "vol-0abc123"
```

Pass a pointer to a struct — data is written directly into it, like `json.Unmarshal`. Schema is generated from your struct via reflection — `json` tags for field names, `sense` tags for descriptions. Pointer fields are optional; value fields are required.

Works with nested structs, slices, and all Go primitive types.

A generic function is also available for callers who prefer compile-time type safety:

```go
result, err := sense.Extract[MountError](s, "device /dev/sdf already mounted with vol-0abc123").Run()
fmt.Println(result.Data.Device)   // "/dev/sdf"
```

### Use cases

Extract isn't just for tests. Use it anywhere you need structure from messy text:

```go
// Parse log lines into typed events
var event DeployEvent
s.Extract(logLine, &event).Context("Kubernetes deployment logs").Run()

// Classify support tickets
var ticket TicketInfo
s.Extract(emailBody, &ticket).Context("Customer support emails for a SaaS product").Run()

// Normalize inconsistent API responses
var order Order
s.Extract(thirdPartyJSON, &order).Context("Legacy vendor API, format varies by region").Run()
```

## Judge — evaluate non-deterministic output

### Assert — test assertion, continues on failure

```go
func TestMyAgent(t *testing.T) {
    output := runMyAgent()

    sense.Assert(t, output).
        Expect("produces valid Go code").
        Expect("handles errors idiomatically").
        Context("task was to write a REST API server").
        Run()
}
```

When a check fails, you get structured feedback — what passed, what failed, why, and evidence:

```
--- FAIL: TestMyAgent (4.82s)
    agent_test.go:15: evaluation: 1/2 passed, score: 0.50

        ✓ produces valid Go code
          reason: The snippet is syntactically valid Go code for a simple addition function.
          evidence: func Add(a, b int) int { return a + b }
          confidence: 0.95

        ✗ handles errors idiomatically
          reason: The output is a trivial math function with no error handling whatsoever.
            It does not demonstrate idiomatic Go error handling (e.g., returning an error
            as a second value, using fmt.Errorf, etc.), nor does it relate to a REST API
            server where error handling would be expected.
          evidence: func Add(a, b int) int { return a + b } — no error return value,
            no error handling logic, no REST API context
          confidence: 0.99
```

### Require — test assertion, stops on failure

```go
sense.Require(t, output).
    Expect("produces valid Go code").
    Run()
```

`Assert` uses `t.Error()` (test continues). `Require` uses `t.Fatal()` (test stops). Same pattern as testify.

### Eval — inspect results programmatically

```go
result, err := sense.Eval(output).
    Expect("is a complete sentence").
    Expect("mentions an animal").
    Expect("contains a number").
    Judge()

fmt.Println(result.Pass)   // false
fmt.Println(result.Score)  // 0.67

for _, c := range result.FailedChecks() {
    fmt.Println(c.Expect, "—", c.Reason)
}
```

### Compare — A/B test two outputs

```go
cmp, err := sense.Compare(outputV1, outputV2).
    Criteria("completeness").
    Criteria("clarity").
    Criteria("professionalism").
    Judge()

fmt.Println(cmp.Winner)     // "A"
fmt.Println(cmp.ScoreA)     // 0.85
fmt.Println(cmp.ScoreB)     // 0.10
fmt.Println(cmp.Reasoning)  // "Output A is significantly better..."
```

## Session

Three tiers — use only what you need:

```go
// Zero config — just works
sense.Assert(t, output).Expect("covers all sections").Run()

// Test suite — auto-cleanup, usage tracking
s := sense.ForTest(t)
s.Assert(t, output).Expect("covers all sections").Run()

// Custom config
s := sense.New(sense.WithModel("claude-haiku-4-5-20251001"))
s.Assert(t, output).Expect("covers all sections").Run()
```

Extract requires an explicit session:

```go
s := sense.New()
var m MountError
s.Extract("device /dev/sdf already mounted", &m).Run()

// Generic version
result, err := sense.Extract[MountError](s, logLine).Run()
```

### Functional options

```go
s := sense.New(
    sense.WithModel("claude-haiku-4-5-20251001"),
    sense.WithTimeout(10 * time.Second),
    sense.WithRetries(5),
    sense.WithAPIKey("sk-..."),
    sense.WithCache(sense.MemoryCache()),
)
```

### ForTest — auto-cleanup for test suites

```go
s := sense.ForTest(t)                                    // defaults
s := sense.ForTest(t, sense.WithModel("claude-haiku-4-5-20251001"))  // custom

// t.Cleanup handles Close and prints usage summary
```

### Usage tracking

```go
s := sense.New()
// ... run evaluations ...
fmt.Println(s.Usage())
// sense: 15 calls, 18420 input tokens, 4210 output tokens
```

Token usage is tracked across all operations using atomic counters — safe for concurrent use.

## Batching

Enable batching for 50% cost reduction. Requests are collected and submitted as a single Anthropic Batch API call:

```go
s := sense.New(sense.WithBatch(50, 2*time.Second))
defer s.Close() // required — flushes pending batch requests
```

**Note:** Batching trades latency for cost. The Batch API processes requests asynchronously — it can take minutes to hours depending on load. Use it for large test suites where 50% cost savings matter more than speed.

## Running Tests

Unit tests use a mock caller and don't hit the API:

```bash
go test ./...
```

E2e tests hit the real Claude API and **cost money** (~$0.10-0.15 per full suite run):

```bash
ANTHROPIC_API_KEY=... go test -tags=e2e -v ./...
```

### Offline Development

Skip all sense calls when you don't have an API key:

```bash
SENSE_SKIP=1 go test ./...
```

All `Assert`, `Require`, `Eval`, `Extract`, and `Compare` calls become no-ops that pass immediately.

## Interfaces

Sense provides two interfaces for decoupling your code from the concrete Session:

```go
// For code that judges output
func AnalyzeReport(s sense.Evaluator, doc string) (bool, error) {
    result, err := s.Eval(doc).
        Expect("has executive summary").
        Judge()
    if err != nil {
        return false, err
    }
    return result.Pass, nil
}

// For code that extracts structure
func ParseTicket(s sense.Extractor, raw string) (*Ticket, error) {
    var t Ticket
    _, err := s.Extract(raw, &t).Run()
    return &t, err
}
```

`*Session` satisfies both interfaces. Accept `Evaluator` or `Extractor` in your function signatures to make your code testable without the Claude API.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | Claude API key | Required |
| `SENSE_MODEL` | Override default judge model | `claude-sonnet-4-6` |
| `SENSE_SKIP` | Set to `1` to skip all sense calls | unset |

## How It Works

1. Your struct schema (Extract) or expectations (Judge) become a prompt
2. Claude is forced to call a structured tool via `tool_choice`
3. The tool's input schema enforces the output format server-side
4. Sense unmarshals the tool call result into typed Go structs

No prompt engineering. No JSON parsing. No "hope the model returns valid output." The schema is enforced server-side.

## What's Next

- [ ] **Deterministic checks** — mix `Check(sense.ValidJSON())` with LLM-judged `Expect()` in the same assertion. Deterministic checks run first; if any fail, skip the LLM call. Free, fast, saves money.
- [ ] **Extract validation** — `Validate(func(T) error)` on extracted structs. Catch hallucinated values (negative totals, impossible dates) without another LLM call.
- [ ] **File cache** — cache responses to disk. Identical prompts during iterative development hit the cache instead of the API.
- [ ] **Prompt caching** — use Anthropic's `cache_control` to reduce cost on repeated system prompts within a session.
- [ ] **Snapshots** — save eval results to disk, detect regressions when prompts change. `SENSE_UPDATE_SNAPSHOTS=1` to update.
- [ ] **CI reporter** — JUnit XML output and GitHub Actions annotations so eval results show up in your pipeline.
- [ ] **Multi-judge consensus** — fan out to N models, require agreement for a pass. Reduces false positives from single-model bias.
- [ ] **ExtractSlice[T]** — extract `[]T` from text with multiple items (invoices, log batches, entity lists).
- [ ] **Cost budget** — `MaxCost: sense.Dollars(0.50)` to cap session spend. Prevents runaway costs in CI.

These are ideas, not commitments. See [docs/NEXT.md](docs/NEXT.md) for details.
