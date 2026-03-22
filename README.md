# Sense

Make sense of non-deterministic output. Agent-powered assertions for Go tests.

```go
func TestAgentProducesDoc(t *testing.T) {
    doc := runMyAgentE2E()

    sense.Assert(t, doc).
        Expect("covers all sections from the brief").
        Expect("includes actionable recommendations").
        Expect("does not hallucinate data sources").
        Run()
}
```

## Why

You're testing agents. Agents produce non-deterministic output. You can't `assert.Equal`. You can't `assert.Contains` because format varies. You need another agent to judge whether the output meets fuzzy, semantic requirements.

## Install

```bash
go get github.com/itsHabib/sense
```

Set your API key:

```bash
export ANTHROPIC_API_KEY=...
```

## Usage

### Assert — fail tests on fuzzy expectations

```go
import "github.com/itsHabib/sense"

func TestMyAgent(t *testing.T) {
    output := runMyAgent()

    sense.Assert(t, output).
        Expect("produces valid Go code").
        Expect("handles errors idiomatically").
        Context("task was to write a REST API server").
        Run()
}
```

Calls `t.Fatal()` on failure with detailed output:

```
--- FAIL: TestMyAgent (2.34s)
    agent_test.go:15: agent assertion failed (1/2 passed, score: 0.50)

        ✓ produces valid Go code
          reason: output contains syntactically valid Go with package declaration
          confidence: 0.95

        ✗ handles errors idiomatically
          reason: no error handling found — missing if err != nil pattern
          evidence: function returns (string) instead of (string, error)
          confidence: 0.92
```

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

## Configuration

```go
func TestMain(m *testing.M) {
    sense.Configure(sense.Config{
        APIKey:     os.Getenv("ANTHROPIC_API_KEY"), // default: $ANTHROPIC_API_KEY
        Model:      "claude-sonnet-4-6",            // default
        Timeout:    30 * time.Second,               // default
        MaxRetries: 3,                              // default
    })
    os.Exit(m.Run())
}
```

## Offline Development

Skip all agent assertions when you don't have an API key:

```bash
SENSE_SKIP=1 go test ./...
```

All `Assert` and `Eval` calls become no-ops that pass immediately.

## How It Works

1. Your expectations become a numbered list in a prompt
2. Claude is forced to call a `submit_evaluation` tool via `tool_choice`
3. The tool's input schema enforces structured JSON (pass/fail per expectation, confidence, reason, evidence)
4. Sense unmarshals the tool call result and either passes the test or calls `t.Fatal()`

No prompt engineering. No JSON parsing. No "hope the model returns valid output." The schema is enforced server-side.
