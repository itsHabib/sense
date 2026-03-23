# Sense

Make sense of non-deterministic output. Evaluate, compare, and extract structured data from text using Claude.

```go
s := sense.NewSession(sense.Config{})
defer s.Close()

// Judge agent output against expectations
s.Assert(t, doc).
    Expect("covers all sections from the brief").
    Expect("includes actionable recommendations").
    Run()

// Parse unstructured text into typed structs
result, err := sense.Extract[MountError](s,
    "device /dev/sdf already mounted with vol-0abc123").Run()
fmt.Println(result.Data.Device)   // "/dev/sdf"
fmt.Println(result.Data.VolumeID) // "vol-0abc123"
```

## Why

You're working with non-deterministic output — from agents, LLMs, logs, error messages, APIs. You can't `assert.Equal`. You can't regex your way through every format variation. You need structured judgment and extraction.

Sense uses the [Anthropic API](https://docs.anthropic.com/en/docs) (Claude) to evaluate and extract. It forces structured responses via Claude's `tool_use` feature — no prompt engineering, no JSON parsing on your end. Requires an Anthropic API key.

## Install

```bash
go get github.com/itsHabib/sense
```

Set your API key:

```bash
export ANTHROPIC_API_KEY=...
```

## Usage

Create a session once, reuse it everywhere:

```go
s := sense.NewSession(sense.Config{})
defer s.Close()
```

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
--- FAIL: TestMyAgent (2.34s)
    agent_test.go:15: evaluation: 1/2 passed, score: 0.50

        ✓ produces valid Go code
          reason: output contains syntactically valid Go with package declaration
          confidence: 0.95

        ✗ handles errors idiomatically
          reason: no error handling found — missing if err != nil pattern
          evidence: function returns (string) instead of (string, error)
          confidence: 0.92
```

### Extract — parse unstructured text into Go structs

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

Works with nested structs, slices, and all Go primitive types. Not just for testing — use it anywhere you need structure from messy text (logs, error messages, API responses, support tickets).

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

### Usage tracking

```go
fmt.Println(s.Usage())
// sense: 15 calls, 18420 input tokens, 4210 output tokens
```

Token usage is tracked across all operations using atomic counters — safe for concurrent use.

## Configuration

```go
s := sense.NewSession(sense.Config{
    APIKey:     os.Getenv("ANTHROPIC_API_KEY"), // default: $ANTHROPIC_API_KEY
    Model:      "claude-sonnet-4-6",            // default
    Timeout:    30 * time.Second,               // default
    MaxRetries: 3,                              // default
})
defer s.Close()
```

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

All `Assert`, `Require`, `Eval`, and `Extract` calls become no-ops that pass immediately.

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

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | Claude API key | Required |
| `SENSE_MODEL` | Override default judge model | `claude-sonnet-4-6` |
| `SENSE_SKIP` | Set to `1` to skip all sense calls | unset |

## How It Works

1. Your expectations (or struct schema) become a prompt
2. Claude is forced to call a structured tool via `tool_choice`
3. The tool's input schema enforces the output format server-side
4. Sense unmarshals the tool call result into typed Go structs

No prompt engineering. No JSON parsing. No "hope the model returns valid output." The schema is enforced server-side.
