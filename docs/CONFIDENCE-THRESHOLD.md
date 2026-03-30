# Confidence Threshold — Design Doc

## The Problem

Sense already returns a per-check `Confidence` score (0.0–1.0) from Claude, but it's purely informational. The `Assert`/`Require` builders only look at the boolean `Pass` field — if Claude says "pass", the check passes, regardless of how uncertain Claude was about that judgment.

This creates a blind spot: a check can pass with 0.3 confidence. Claude is basically saying "I guess so?" and the test treats that the same as a 0.95 "definitely yes." In practice, low-confidence passes correlate with ambiguous expectations or borderline outputs — exactly the cases where you want the test to flag the result for review.

## What Exists Today

The tool schema already **requires** Claude to return confidence:

```json
{
  "expect": "describes a multi-week training program",
  "pass": true,
  "confidence": 0.45,
  "reason": "The output mentions a 12-week timeline but only describes week 1 in detail"
}
```

The `Check` struct stores it:

```go
type Check struct {
    Expect     string  `json:"expect"`
    Pass       bool    `json:"pass"`
    Confidence float64 `json:"confidence"`
    Reason     string  `json:"reason"`
    Evidence   string  `json:"evidence,omitempty"`
}
```

The `EvalResult.String()` method already prints it:

```
✓ describes a multi-week training program
  reason: mentions 12-week timeline but only details week 1
  confidence: 0.45
```

But nothing acts on it. The data flows through the system and gets discarded at the decision point.

## Proposal

Add a `MinConfidence` option to `AssertBuilder` and `EvalBuilder` that treats low-confidence passes as failures.

### API

```go
// Per-assertion threshold
s.Assert(t, output).
    Expect("includes specific exercises with sets and reps").
    MinConfidence(0.7).
    Run()

// Per-session default (applies to all assertions unless overridden)
s := sense.New(sense.WithMinConfidence(0.7))
s.Assert(t, output).Expect("...").Run()

// Override session default for a specific assertion
s.Assert(t, output).
    Expect("mentions progressive overload").
    MinConfidence(0.5). // relax for this one
    Run()

// Eval builder (programmatic)
result, err := s.Eval(output).
    Expect("calorie targets are provided").
    MinConfidence(0.8).
    Judge()
// result.Pass is false if any check passed with confidence < 0.8
```

### Behavior

When `MinConfidence` is set, the pass/fail decision for each check becomes:

```
check passes = check.Pass && check.Confidence >= minConfidence
```

The `EvalResult` fields update accordingly:
- `result.Pass` — true only if ALL checks pass (including confidence threshold)
- `result.Score` — fraction of checks that pass the threshold
- `result.Checks[i].Pass` — **unchanged** (still reflects Claude's raw judgment)

A new field on `Check` indicates when confidence caused the failure:

```go
type Check struct {
    // ... existing fields ...
    BelowThreshold bool `json:"below_threshold,omitempty"`
}
```

The `String()` output makes it clear:

```
✗ includes specific exercises with sets and reps
  reason: mentions "compound movements" but doesn't list specific exercises
  confidence: 0.42 (below threshold 0.70)
```

### Why Not Modify Check.Pass?

Keeping `Check.Pass` as Claude's raw judgment preserves the original signal. The threshold is a consumer-side policy, not a change in what Claude thinks. This means:

- `result.Checks[i].Pass` — "Claude thinks this passes"
- `result.Pass` — "Claude thinks this passes AND is confident enough"
- `result.Checks[i].BelowThreshold` — "passed Claude's judgment but didn't meet your confidence bar"

Users can inspect both dimensions independently.

## Implementation

### Changes to existing types

**`option.go`** — add session-level option:

```go
type sessionConfig struct {
    // ... existing fields ...
    minConfidence    float64
    minConfidenceSet bool
}

func WithMinConfidence(threshold float64) Option {
    return func(c *sessionConfig) {
        c.minConfidence = threshold
        c.minConfidenceSet = true
    }
}
```

**`config.go` / `sense.go`** — store on Session:

```go
type Session struct {
    // ... existing fields ...
    minConfidence float64
}
```

**`eval.go`** — add to `EvalBuilder`:

```go
type EvalBuilder struct {
    // ... existing fields ...
    minConfidence    float64
    minConfidenceSet bool
}

func (b *EvalBuilder) MinConfidence(threshold float64) *EvalBuilder {
    b.minConfidence = threshold
    b.minConfidenceSet = true
    return b
}
```

Apply threshold after getting Claude's response in `JudgeContext`:

```go
// After json.Unmarshal(raw, &result):
threshold := b.minConfidence
if !b.minConfidenceSet {
    threshold = b.session.minConfidence
}

if threshold > 0 {
    allPass := true
    passCount := 0
    for i := range result.Checks {
        if result.Checks[i].Pass && result.Checks[i].Confidence < threshold {
            result.Checks[i].BelowThreshold = true
            allPass = false
        } else if result.Checks[i].Pass {
            passCount++
        }
    }
    result.Pass = allPass
    result.Score = float64(passCount) / float64(len(result.Checks))
}
```

**`assert.go`** — add to `AssertBuilder`:

```go
func (b *AssertBuilder) MinConfidence(threshold float64) *AssertBuilder {
    b.eval.MinConfidence(threshold)
    return b
}
```

**`eval.go`** — update `Check` type and `String()`:

```go
type Check struct {
    // ... existing fields ...
    BelowThreshold bool `json:"below_threshold,omitempty"`
}

// In String():
if c.BelowThreshold {
    fmt.Fprintf(&b, "      confidence: %.2f (below threshold)\n", c.Confidence)
}
```

### Files changed

| File | Change |
|------|--------|
| `option.go` | Add `WithMinConfidence` option |
| `config.go` | Store `minConfidence` on Session |
| `eval.go` | Add `MinConfidence` to EvalBuilder, apply threshold in `JudgeContext`, add `BelowThreshold` to Check |
| `assert.go` | Add `MinConfidence` passthrough to AssertBuilder |

### What doesn't change

- The Claude tool schema — confidence is already required
- The system prompt — Claude already provides confidence scores
- The `Compare` builder — confidence isn't relevant for A/B comparison
- The `Extract` builder — no pass/fail concept

## Edge Cases

**Threshold = 0.0 (or unset):** No behavior change. All passes are accepted regardless of confidence. This is the current default.

**Threshold = 1.0:** Only checks where Claude is fully certain pass. In practice, Claude rarely returns exactly 1.0, so this is very strict. Probably not useful but not harmful.

**Claude returns Pass=false with high confidence:** Threshold doesn't apply — the check already failed. `BelowThreshold` is only set when `Pass=true && Confidence < threshold`.

**All checks fail confidence threshold:** `result.Pass = false`, `result.Score = 0.0`. The test fails with a clear message showing each check's confidence vs. threshold.

## Testing

1. Unit test: mock Claude returning checks with varying confidence levels, verify threshold logic
2. Unit test: session-level default is overridden by builder-level setting
3. Unit test: `BelowThreshold` field set correctly
4. Unit test: `EvalResult.String()` includes threshold annotation
5. E2E test: real Claude call with a deliberately ambiguous expectation, verify low confidence is returned and threshold catches it

## Open Questions

- **Should `Compare` support confidence thresholds?** The compare builder returns `CriterionResult` which has per-criterion scores but no confidence field. Could add it, but the use case is less clear.
- **Should `BelowThreshold` appear in the JSON output?** Currently proposed as `omitempty` — absent when false. Alternative: always present for consistency.
- **Default threshold for `ForTest`?** Could set a reasonable default (e.g., 0.6) for the test helper path since tests should be stricter. But changing defaults is a breaking change for v0.4 users.
