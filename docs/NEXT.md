# Sense — Next Steps

Four features to take sense from "works" to "production-grade eval framework."

---

## 1. Cost Tracking

Track token usage across a session and surface it to the user.

**API:**

```go
s := sense.NewSession(sense.Config{})
defer s.Close()

// ... run tests ...

fmt.Println(s.Usage())
// sense: 15 calls, 18,420 input tokens, 4,210 output tokens, ~$0.03
```

**What to build:**

- `Session.usage` — atomic counters for input/output tokens and call count
- Accumulate from every `callRequest` result (both individual and batch)
- `Usage()` method returns a `SessionUsage` struct with a `String()` that includes estimated cost
- Optional: print summary automatically in `Close()` if a verbose flag is set

**Why it matters:**

Right now you have no idea what a test suite costs. One bad prompt with a huge context window could 10x your bill. This makes cost visible so you can optimize.

---

## 2. Snapshots

Save eval results to disk. On subsequent runs, compare against the snapshot. Catch regressions in your prompts or model behavior.

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

**Why it matters:**

You change a system prompt, run your evals, everything passes. But did quality actually degrade? Without snapshots you'd never know that "handles errors idiomatically" went from 0.95 confidence to 0.60. Snapshots make prompt regressions visible.

---

## 3. Multi-Judge Consensus

Run the same evaluation against multiple models. Require agreement for a pass. Reduces false positives from a single model's biases.

**API:**

```go
s := sense.NewSession(sense.Config{
    Consensus: &sense.ConsensusConfig{
        Models:   []string{"claude-sonnet-4-6", "claude-haiku-4-5-20251001"},
        Strategy: sense.ConsensusAll,  // all must agree
    },
})

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

- `ConsensusConfig` struct on `Config` — models list + strategy
- When consensus is configured, `JudgeContext` fans out to N models concurrently
- Each model gets the same prompt, returns its own `EvalResult`
- Merge results based on strategy: aggregate pass/fail, average scores, combine reasons
- `EvalResult` gets a `Judgments []ModelJudgment` field for per-model breakdown
- Works with both individual calls and batching

**Why it matters:**

A single model can have blind spots. Haiku might miss a subtle factual error that Sonnet catches. Running both and requiring agreement gives you higher confidence in your evals. The cost is 2x but the reliability gain is significant for critical assertions.

---

## 4. Custom Deterministic Evaluators

Let users register their own evaluators that run locally — no API call. Mix deterministic checks (regex, JSON schema, word count) with LLM-judged semantic checks in the same assertion.

**API:**

```go
s.Assert(t, output).
    Expect("is valid JSON").
    Expect("has a 'status' field").        // deterministic
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
- Deterministic checks run first (fast, free). If any fail, skip the LLM call entirely — saves cost
- Results merge into the same `EvalResult.Checks` slice with `Confidence: 1.0`

**Why it matters:**

Not everything needs an LLM judge. "Is this valid JSON?" is a `json.Unmarshal` call, not a $0.003 API request. Mixing deterministic and semantic checks in one assertion gives you fast feedback on the obvious stuff and LLM judgment on the fuzzy stuff. Running deterministic checks first also lets you fail fast — no point asking Claude if the output "handles errors well" when it's not even valid Go.

---

## Build Order

1. **Cost Tracking** — smallest scope, immediately useful, touches only Session
2. **Custom Deterministic Evaluators** — high value, saves API costs, no new external dependencies
3. **Snapshots** — needs file I/O and diffing logic, but self-contained
4. **Multi-Judge Consensus** — most complex, multiplies API calls, needs concurrency work

Each feature is independent — they can be built and shipped in any order.
