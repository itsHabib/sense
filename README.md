# Sense

Make sense of non-deterministic output. Extract structured data from text and evaluate output quality using Claude.

```go
s := sense.NewSession(sense.Config{})
defer s.Close()

// Extract: unstructured text → typed struct
result, err := sense.Extract[MountError](s,
    "device /dev/sdf already mounted with vol-0abc123").Run()
fmt.Println(result.Data.Device)   // "/dev/sdf"
fmt.Println(result.Data.VolumeID) // "vol-0abc123"

// Judge: output → pass/fail with evidence
s.Assert(t, doc).
    Expect("covers all sections from the brief").
    Expect("includes actionable recommendations").
    Run()
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

result, err := sense.Extract[MountError](s,
    "device /dev/sdf already mounted with vol-0abc123").
    Context("AWS EC2 EBS error messages").
    Run()

fmt.Println(result.Data.Device)   // "/dev/sdf"
fmt.Println(result.Data.VolumeID) // "vol-0abc123"
```

Schema is generated from your struct via reflection — `json` tags for field names, `sense` tags for descriptions. Pointer fields are optional; value fields are required.

Works with nested structs, slices, and all Go primitive types.

### Use cases

Extract isn't just for tests. Use it anywhere you need structure from messy text:

```go
// Parse log lines into typed events
event, _ := sense.Extract[DeployEvent](s, logLine).
    Context("Kubernetes deployment logs").Run()

// Classify support tickets
ticket, _ := sense.Extract[TicketInfo](s, emailBody).
    Context("Customer support emails for a SaaS product").Run()

// Normalize inconsistent API responses
order, _ := sense.Extract[Order](s, thirdPartyJSON).
    Context("Legacy vendor API, format varies by region").Run()
```

## Judge — evaluate non-deterministic output

### Assert — test assertion, continues on failure

```go
func TestMyAgent(t *testing.T) {
    output := runMyAgent()

    s.Assert(t, output).
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
s.Require(t, output).
    Expect("produces valid Go code").
    Run()
```

`Assert` uses `t.Error()` (test continues). `Require` uses `t.Fatal()` (test stops). Same pattern as testify.

### Eval — inspect results programmatically

```go
result, err := s.Eval(output).
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
cmp, err := s.Compare(outputV1, outputV2).
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

Create a session once, reuse it everywhere:

```go
s := sense.NewSession(sense.Config{
    APIKey:     os.Getenv("ANTHROPIC_API_KEY"), // default: $ANTHROPIC_API_KEY
    Model:      "claude-sonnet-4-6",            // default
    Timeout:    30 * time.Second,               // default
    MaxRetries: 3,                              // default
})
defer s.Close()
```

### Usage tracking

```go
fmt.Println(s.Usage())
// sense: 15 calls, 18420 input tokens, 4210 output tokens
```

Token usage is tracked across all operations using atomic counters — safe for concurrent use.

## Batching

Enable batching for 50% cost reduction. Requests are collected and submitted as a single Anthropic Batch API call:

```go
s = sense.NewSession(sense.Config{
    Batch: &sense.BatchConfig{
        MaxSize: 50,                   // flush after 50 requests
        MaxWait: 2 * time.Second,      // or after 2s, whichever first
    },
})
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
