# Sense Integration Feedback

Friction points and improvement ideas from a production integration that uses sense heavily for structured extraction across a multi-agent pipeline. Ordered by estimated impact.

---

## 1. Per-Call Model & Timeout Override

**Problem:** When an app needs both fast user-facing calls (Haiku, 30s) and slower background extraction (Sonnet, 2min), it must create **two separate sessions** — solely because model and timeout differ. This doubles cleanup, nil-checking, and wiring.

```go
// Current: two sessions
fast := sense.New(sense.WithModel("claude-haiku-4-5-20251001"), sense.WithTimeout(30*time.Second))
deep := sense.New(sense.WithModel("claude-sonnet-4-6"), sense.WithTimeout(2*time.Minute))

// Proposed: one session, override per call
s := sense.New(sense.WithModel("claude-sonnet-4-6"))
s.Extract(text, &dst).Model("claude-haiku-4-5-20251001").Timeout(30*time.Second).RunContext(ctx)
```

---

## 2. Parallel / Concurrent Extraction

**Problem:** When extracting multiple struct types from different texts (e.g., 5 agent outputs + shared coordinator output), each is a separate sequential API call — 8+ serial round-trips. A concurrent extraction helper would cut wall-clock time dramatically.

```go
// Current: sequential in a loop
for _, output := range outputs {
    sess.Extract(output.Content, &agentExtract).RunContext(ctx)
}
sess.Extract(sharedContent, &scheduleExtract).RunContext(ctx)
sess.Extract(sharedContent, &summaryExtract).RunContext(ctx)

// Proposed: parallel extraction
results := sense.ExtractParallel(ctx, sess, []sense.ExtractJob{
    {Text: output1, Dest: &agent1},
    {Text: output2, Dest: &agent2},
    {Text: sharedContent, Dest: &schedule},
})
```

---

## 3. No-Op / Passthrough Session

**Problem:** Callers must guard every sense call with `if s.sense != nil` because the session may not be configured (no API key, test env, CI). A built-in no-op session would eliminate all nil checks.

```go
// Current: nil checks scattered across multiple files
if s.sense != nil {
    result, err := s.sense.Extract(text, &dst).RunContext(ctx)
    ...
}

// Proposed: sense.Nop() returns zero values, no API calls
s := sense.Nop()
// — or auto-degrade when ANTHROPIC_API_KEY is unset —
s := sense.New() // returns nop session if no key
```

---

## 4. Validation Hooks / Post-Extraction Checks

**Problem:** After extraction, callers manually validate results — e.g., filtering hallucinated IDs against an allow-list, checking numeric invariants (`field_a + field_b == 7`), verifying values are positive. This logic lives outside sense and is easy to forget.

```go
// Current: manual post-validation
allowedSet := map[string]bool{...}
for _, item := range extracted.Items {
    if !allowedSet[item.ID] { /* filter out */ }
}

// Proposed: implement a Validator interface, sense calls it automatically
type selection struct {
    Items []Item `json:"items"`
}

func (s *selection) Validate() error {
    for _, item := range s.Items {
        if !allowedIDs[item.ID] {
            return fmt.Errorf("unknown ID: %s", item.ID)
        }
    }
    return nil
}
// sense.Extract calls Validate() after unmarshalling
```

---

## 5. Observability / Logging Hooks

**Problem:** Apps with structured logging (`slog`) get no visibility into sense internals — prompts sent, per-call latency, token usage, or errors. Sense is a black box, which makes debugging extraction failures hard.

```go
// Proposed: logger option
sense.New(
    sense.WithLogger(slog.Default()),
)

// Or a hook interface for metrics/tracing
sense.New(
    sense.WithHook(func(event sense.Event) {
        // event.Op, event.Model, event.Duration, event.Tokens, event.Err
    }),
)
```

---

## 6. Session-Level Default Context

**Problem:** Every `.Extract()` call passes `.Context(...)` with similar preambles. A session-level context would reduce repetition and ensure consistency.

```go
// Current: repeated per call
sess.Extract(content, &report).Context("This is the report-writer's output...").RunContext(ctx)
sess.Extract(content, &agent).Context("This is an agent's output...").RunContext(ctx)

// Proposed: session-level context, per-call context appends
sess := sense.New(
    sense.WithContext("You are extracting structured data from a multi-agent pipeline output."),
)
sess.Extract(content, &report).Context("Specifically the report-writer section.").RunContext(ctx)
```

---

## 7. Error Type Granularity

**Problem:** When extraction fails and the caller wants to fall back gracefully, it can't distinguish failure modes. "API timeout" (retry with longer timeout), "model confused" (simpler struct needed), and "rate limited" (exponential backoff) all look the same. The sentinel errors exist (`ErrRateLimit`, `ErrNoToolCall`, etc.) but aren't consistently surfaced through the builder's `RunContext`.

```go
// Current: all errors treated the same
if err != nil {
    logger.Warn("sense extraction failed, falling back", "error", err)
}

// Desired: smarter fallback logic
if errors.Is(err, sense.ErrRateLimit) { /* back off */ }
if errors.Is(err, sense.ErrTimeout) { /* retry with longer timeout */ }
if errors.Is(err, sense.ErrNoToolCall) { /* model confused, try simpler struct */ }
```

---

## 8. Extract-With-Fallback Pattern

**Problem:** A common pattern is "try sense, fall back to regex/manual parsing on failure." A built-in fallback mechanism would formalize this and reduce boilerplate.

```go
// Current: repeated try/catch pattern
result, err := sess.Extract(text, &dst).RunContext(ctx)
if err != nil {
    dst = regexFallback(text)
}

// Proposed: built-in fallback
sess.Extract(text, &dst).
    Fallback(func() error { return regexFallback(text, &dst) }).
    RunContext(ctx)
```

---

## 9. Cost Awareness

**Problem:** Production apps run extraction on every request — potentially many calls. Sense tracks `TokensUsed` per result and `SessionUsage` aggregates, but there's no cost estimation. Even a rough `EstimatedCost()` method on `SessionUsage` (using known per-model pricing) would help with monitoring, alerting, and budget guardrails.

---

## Integration Profile

For context, here's the shape of the integration this feedback is based on:

| Pattern | Model | Timeout | Fallback |
|---------|-------|---------|----------|
| Dynamic routing (user-facing) | Haiku | 30s | Heuristic rules → static mapping |
| Intake review (user-facing) | Haiku | 30s | Skip (return empty) |
| Agent output extraction (background, N agents) | Sonnet | 2min | Regex on delimited blocks |
| Schedule extraction (background) | Sonnet | 2min | Regex on delimited blocks |
| Structured data extraction (background) | Sonnet | 2min | Regex on JSON blocks |
| E2E test assertions | Default | Default | N/A (test failure) |

**Files importing sense:** 8
**Extraction target structs:** 7+ (with nested subtypes)
**Fallback layers:** 3 (sense → heuristic/regex → static defaults)
