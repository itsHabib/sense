# Technical Design

## How It Works

```
Test code                         AgentKit                          Claude API
─────────                         ────────                          ─────────
agent.Assert(t, doc)         →    Build EvalBuilder
  .Expect("has intro")       →    Collect expectations
  .Expect("has conclusion")  →    Collect expectations
  .Run()                     →    Construct prompt              →   API call (tool_use)
                                  Unmarshal tool call result     ←   EvalResult JSON
                                  Check all passes
                                  t.Fatal() if any fail         →   Test fails/passes
```

### Step 1: Prompt Construction

The SDK builds a system prompt and user message from the expectations and output:

**System prompt:**
```
You are a strict test evaluator. You will receive output to evaluate
and a list of expectations. For each expectation, determine whether
the output satisfies it.

Be strict. Only pass an expectation if you are confident the output
satisfies it. When in doubt, fail it and explain why.
```

**User message:**
```
Output to evaluate:
"""
{the agent's output}
"""

Expectations:
1. has intro
2. has conclusion

Evaluate each expectation and submit your result.
```

### Step 2: Structured Output via Tool Use

The SDK defines a tool whose input schema matches `EvalResult`:

```json
{
  "name": "submit_evaluation",
  "description": "Submit the evaluation result",
  "input_schema": {
    "type": "object",
    "properties": {
      "pass": {"type": "boolean"},
      "score": {"type": "number", "minimum": 0, "maximum": 1},
      "checks": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "expect": {"type": "string"},
            "pass": {"type": "boolean"},
            "confidence": {"type": "number", "minimum": 0, "maximum": 1},
            "reason": {"type": "string"},
            "evidence": {"type": "string"}
          },
          "required": ["expect", "pass", "confidence", "reason"]
        }
      }
    },
    "required": ["pass", "score", "checks"]
  }
}
```

The request uses `tool_choice: {"type": "tool", "name": "submit_evaluation"}` to force the model to call this tool. The server enforces the schema — no parsing failures.

### Step 3: Result Extraction

The tool call's input arguments are JSON. Unmarshal into `EvalResult`. Check if all expectations passed. If any failed, format a useful error message and call `t.Fatal()`.

### Test Output on Failure

```
--- FAIL: TestAgentProducesDoc (2.34s)
    agent_test.go:15: agent assertion failed (2/3 passed, score: 0.67)

        ✓ covers all sections from the brief
          reason: output contains Introduction, Analysis, and Recommendations sections
          confidence: 0.95

        ✗ includes actionable recommendations
          reason: recommendations section contains only general advice without specific next steps
          evidence: "Consider improving your process" — not actionable
          confidence: 0.88

        ✓ does not hallucinate data sources
          reason: all cited sources appear in the provided input data
          confidence: 0.92
```

### Caching

Every API call is cacheable. The cache key is SHA-256 of:
- Model name
- System prompt
- User message (includes output + expectations)

Cache is opt-in. When enabled, the first run hits the API and saves the response. Subsequent runs read from cache — free, fast, deterministic.

```go
func TestMain(m *testing.M) {
    agent.Configure(agent.Config{
        Cache: agent.FileCache("testdata/agent-cache"),
    })
    os.Exit(m.Run())
}
```

Cache files live in `testdata/` and are committed to the repo. CI runs are free after the first run.

**Cache invalidation:** Delete the cache directory when you change the agent under test. The cache key includes the output, so if the agent produces different output, it's a cache miss anyway.

### Quick Mode

For expectations that don't need an AI judge:

```go
agent.Assert(t, doc).
    Quick().
    Expect("contains ## Introduction").
    Expect("is not empty").
    Expect("has more than 5 lines").
    Run()
```

Quick mode uses string matching and regex — no API call, sub-millisecond, free. Supported patterns:

| Pattern | Implementation |
|---------|---------------|
| `"contains X"` | `strings.Contains` |
| `"starts with X"` | `strings.HasPrefix` |
| `"ends with X"` | `strings.HasSuffix` |
| `"matches REGEX"` | `regexp.MatchString` |
| `"is not empty"` | `len(strings.TrimSpace) > 0` |
| `"is valid JSON"` | `json.Valid` |
| `"has more than N lines"` | line count |

Anything else in Quick mode returns an error telling you to use Standard mode.

### Consensus Mode

For high-stakes assertions, run N independent judges and take majority vote:

```go
agent.Assert(t, doc).
    Consensus(3).
    Expect("is production-ready documentation").
    Run()
```

3 independent API calls, run concurrently. Passes if 2/3 agree it passes.

### Retry & Error Handling

- Rate limit (429): exponential backoff, respect `retry-after`
- Server error (5xx): retry up to 3 times
- Timeout: configurable, default 30s
- Schema mismatch: retry with error feedback (almost never happens with tool_use)
- Auth error (401/403): fail immediately with clear message

All errors surface through `t.Fatal()` with context:

```
--- FAIL: TestMyAgent (30.12s)
    agent_test.go:15: agent assertion error: rate limited after 3 retries
```
