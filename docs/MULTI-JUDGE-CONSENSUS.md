# Multi-Judge Consensus — Design & Plan

## Problem

A single LLM judge has bias. Sonnet might pass something Haiku would fail, or vice versa. For high-stakes evals (production gates, CI pipelines), you want multiple models to agree before calling something a pass.

## API

### Session-level consensus

```go
s := sense.New(sense.WithConsensus(
    sense.ConsensusAll,
    "claude-sonnet-4-6", "claude-haiku-4-5-20251001",
))

// Every Judge() call fans out to both models
result, err := s.Eval(output).
    Expect("is factually accurate").
    Judge()

fmt.Println(result.Pass)       // true only if ALL models agree
fmt.Println(result.Score)      // average across models
fmt.Println(result.Judgments)   // per-model breakdown
```

### Per-call override

```go
s := sense.New() // no consensus by default

result, err := s.Eval(output).
    Expect("is factually accurate").
    Consensus(sense.ConsensusMajority, "claude-sonnet-4-6", "claude-haiku-4-5-20251001", "claude-opus-4-6").
    Judge()
```

## Strategies

| Strategy | Pass when | Score |
|----------|-----------|-------|
| `ConsensusAll` | Every model passes every check | Average of all model scores |
| `ConsensusMajority` | >50% of models pass each check | Average of all model scores |
| `ConsensusAny` | At least one model passes every check | Max of all model scores |

## Result Types

```go
// Existing EvalResult gains one field:
type EvalResult struct {
    Pass       bool
    Score      float64
    Checks     []Check
    Judgments  []ModelJudgment  // NEW — nil when consensus is not used
    Duration   time.Duration
    TokensUsed int
    Model      string           // "consensus" when multiple models used
    Usage      Usage
}

// Per-model breakdown
type ModelJudgment struct {
    Model      string
    Pass       bool
    Score      float64
    Checks     []Check
    Duration   time.Duration
    TokensUsed int
}
```

When consensus is NOT configured, `Judgments` is nil and everything works exactly as today. Zero breaking changes.

## Implementation Plan

### Step 1: Types & config

- Add `ConsensusStrategy` type (int enum: `ConsensusAll`, `ConsensusMajority`, `ConsensusAny`)
- Add `consensusConfig` struct: `Strategy`, `Models []string`
- Add `WithConsensus(strategy, models...)` option → stores on `sessionConfig`
- Add `Judgments []ModelJudgment` field to `EvalResult`
- Add `ModelJudgment` struct

### Step 2: EvalBuilder plumbing

- Add `consensus *consensusConfig` field to `EvalBuilder`
- Add `Consensus(strategy, models...)` chainable method on `EvalBuilder`
- `AssertBuilder` passes through to its inner `EvalBuilder`
- Session stores `consensusConfig`; `EvalBuilder` inherits it but per-call `.Consensus()` overrides

### Step 3: Fan-out in JudgeContext

This is the core change. When consensus is configured:

```
JudgeContext(ctx)
├─ Build prompt once (outputStr, userMsg — same for all models)
├─ Fan out: for each model, goroutine calls session.client.call() with that model
├─ Collect []EvalResult (one per model)
├─ Merge into single EvalResult using strategy
└─ Return merged result
```

The fan-out reuses the existing `client.call()` path — same prompt, same schema, just different model string. Each goroutine gets its own callRequest. Usage is recorded per-call as usual.

Key decisions:
- **Concurrent**: all models called in parallel via goroutines + errgroup
- **Fail-fast**: if any model errors (not fails — errors), return the error immediately
- **Timeout**: the existing per-call timeout applies to each model call individually

### Step 4: Merge logic

```go
func mergeJudgments(strategy ConsensusStrategy, judgments []ModelJudgment) *EvalResult
```

**ConsensusAll**: For each check index, pass = all models passed that check. Overall pass = all checks pass. Score = average of model scores.

**ConsensusMajority**: For each check index, pass = >50% of models passed that check. Overall pass = all merged checks pass. Score = average.

**ConsensusAny**: For each check index, pass = any model passed that check. Overall pass = all merged checks pass. Score = max of model scores.

Merged checks combine reasons: `"[sonnet] passed: good structure | [haiku] failed: missing conclusion"`.

### Step 5: Compare support

Same pattern for `CompareBuilder.JudgeContext()`. Fan out, collect `[]CompareResult`, merge:
- Winner = majority vote across models
- Scores = averaged
- Per-criterion = majority vote

`CompareResult` gets `Judgments []ModelCompareJudgment`.

### Step 6: Tests

**Unit tests (mock caller)**:
- ConsensusAll: 2 pass → pass, 1 pass + 1 fail → fail
- ConsensusMajority: 2/3 pass → pass, 1/3 pass → fail
- ConsensusAny: 1/3 pass → pass, 0/3 → fail
- Per-call override beats session-level config
- No consensus configured → Judgments is nil, behavior unchanged
- Skip mode works
- Error from one model → error returned
- Usage recorded for all model calls

**E2e tests**:
- Two models evaluate same output, result has Judgments with both models
- Assert with consensus passes/fails correctly

## File Changes

| File | Change |
|------|--------|
| `consensus.go` | NEW — strategy type, config, merge logic |
| `consensus_test.go` | NEW — unit tests for merge strategies |
| `eval.go` | Add `Judgments` to `EvalResult`, fan-out in `JudgeContext` |
| `compare.go` | Same pattern for `CompareBuilder` |
| `assert.go` | Pass-through `Consensus()` method |
| `option.go` | `WithConsensus` option |
| `sense.go` | No change — session methods don't need modification |
| `e2e_test.go` | E2e consensus tests |

## What This Does NOT Include

- **Multi-provider** (OpenAI, Gemini) — that needs a separate `Caller` implementation. This feature is Claude-only, just different Claude models.
- **Weighted voting** — all models have equal weight. Can add later.
- **Per-check thresholds** — pass/fail is binary per strategy. Can add later.
- **Batching + consensus** — works naturally since `batchCaller` satisfies `caller`. Each model call becomes a batch item.

## Open Questions

1. Should `Model()` override be an error when consensus is configured? Or should it be ignored? Current plan: `Model()` is ignored when consensus is active (the models list takes over).
2. Should the merged `Check.Reason` concatenate all model reasons, or just the majority? Current plan: concatenate with model labels.
3. Is `Consensus()` on the builder the right API, or should it be `Models()` + `Strategy()`? Current plan: single `Consensus(strategy, models...)` keeps it atomic.
