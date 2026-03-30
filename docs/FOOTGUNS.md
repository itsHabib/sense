# Footgun Fixes — Implementation Plan

Audit of API design footguns found after implementing confidence threshold and integration feedback features. All items below need fixing.

## 1. ExtractResult[T] missing Usage field

**File:** `extract.go:155-162`

**Problem:** `ExtractResult[T]` has a `Usage` field but it's never populated — only `TokensUsed` is set. Callers reading `result.Usage.InputTokens` get zero silently.

**Fix:** Set `result.Usage = *usage` in the same block that sets `TokensUsed`, matching the pattern in `eval.go:199-200` and `compare.go:128-129`. Also add a `Usage` field to `ExtractResult[T]` if it doesn't exist (check struct definition).

**Also check:** `ExtractIntoResult` and `ExtractSliceResult` for the same omission.

---

## 2. Validator interface only works with ExtractIntoBuilder

**File:** `extractor.go:49-52` (Validator interface), `extract.go` (generic Extract), `extract_slice.go` (ExtractSlice)

**Problem:** The `Validator` interface (`Validate() error`) is checked in `ExtractIntoBuilder.RunContext` after unmarshalling, but NOT in `ExtractBuilder[T].RunContext` or `ExtractSliceBuilder[T].RunContext`. A struct implementing `Validate() error` gets no automatic validation when used with the generic path.

**Fix:** After unmarshalling in `ExtractBuilder[T].RunContext`, check if `data` (of type T) implements `Validator` via `any(data).(Validator)` or `any(&data).(Validator)` (pointer receiver). If so, call it. Same pattern for `ExtractSliceBuilder` — check each item. The existing `.Validate(fn)` closure should run first (explicit beats implicit), then the interface check.

**Note:** For value types, need to check both `any(data)` and `any(&data)` since Validate() is typically on a pointer receiver.

---

## 3. Fallback success returns zero TokensUsed

**Files:** `extract.go:132-139`, `extractor.go:152-158`, `extract_slice.go:133-138`

**Problem:** When the primary API call fails but fallback succeeds, the returned result has `TokensUsed: 0` and no `Usage`. Cost tracking and metrics silently break.

**Fix:** The fallback path already consumed some tokens on the failed attempt (usage may be nil if the call errored before getting a response). The result should at minimum document that it came from fallback. Add a `Fallback bool` field to all three result types (`ExtractResult[T]`, `ExtractIntoResult`, `ExtractSliceResult`). Set it to `true` when the fallback path fires. This way callers can distinguish "zero tokens because fallback" from "zero tokens because bug".

---

## 4. WithTimeout(-1) docs vs implementation mismatch

**File:** `option.go:39-44` (WithTimeout), `option.go:87-89` (applyDefaults)

**Problem:** Doc says "Set to -1 to disable timeouts" but `applyDefaults` converts negative to 0:
```go
if cfg.timeoutSet && cfg.timeout < 0 {
    cfg.timeout = 0
}
```
Then in builders, `if timeout > 0` means 0 = no timeout applied. So -1 technically works (converts to 0, which skips the timeout block). BUT `resolveTimeout` returns 0 when `callSet=false`, and `0 > 0` is false so no timeout — this is correct by accident.

**The real bug:** If a builder calls `.Timeout(-1)` to disable, `timeoutSet=true` and `timeout=-1`. Then `resolveTimeout` returns `-1`. Then `if timeout > 0` is false — so it works. But negative durations flowing through the system is fragile.

**Fix:** In `resolveTimeout`, clamp negative values to 0:
```go
func resolveTimeout(callTimeout time.Duration, callSet bool, sessionTimeout time.Duration) time.Duration {
    t := sessionTimeout
    if callSet {
        t = callTimeout
    }
    if t < 0 {
        return 0
    }
    return t
}
```

And update the doc on `WithTimeout` to be clearer: "Set to -1 or 0 to disable timeouts."

Also add the same doc to builder `.Timeout()` methods.

---

## 5. SENSE_SKIP still fails on validation errors

**Files:** `extractor.go:101-110`, `extract.go:96-103`, `extract_slice.go:97-101`, `eval.go:139-149`, `compare.go:91-96`

**Problem:** When `SENSE_SKIP=1`, pre-flight validation (empty text, bad dest pointer, no expectations) still runs and returns errors. Callers expect skip = always succeed.

**Fix:** Move `shouldSkip()` check BEFORE validation in all builders. The skip check should be the very first thing after method entry. If skip is active, return the skip result immediately without any validation.

Order should be:
1. `shouldSkip()` — return immediately
2. Validate inputs (empty text, nil dest, no expectations)
3. Proceed with API call

---

## 6. Nop() ignores all configuration

**File:** `nop.go:13-17`

**Problem:** `Nop()` creates a bare `Session{}` — no model, no timeout, no context, no logger, no hook, no minConfidence. All options are silently ignored.

**Fix:** Make `Nop()` accept options so callers can configure it if needed:
```go
func Nop(opts ...Option) *Session {
    cfg := &sessionConfig{}
    for _, o := range opts {
        o(cfg)
    }
    applyDefaults(cfg)
    s := &Session{
        client:        &nopCaller{},
        model:         cfg.model,
        timeout:       cfg.timeout,
        maxRetries:    cfg.maxRetries,
        context:       cfg.context,
        minConfidence: cfg.minConfidence,
        logger:        cfg.logger,
        hook:          cfg.hook,
    }
    return s
}
```

This way `sense.Nop()` still works zero-config, but `sense.Nop(sense.WithMinConfidence(0.7))` also works.

---

## 7. Score semantics change with MinConfidence

**File:** `eval.go:219-237` (applyConfidenceThreshold)

**Problem:** Without threshold: `score = fraction of checks that passed (Claude's judgment)`. With threshold: `score = fraction of checks that passed AND were above threshold`. This is a semantic shift — a check that Claude says passes but is below threshold doesn't count toward score at all.

**Current code:**
```go
passCount := 0
for i := range result.Checks {
    if result.Checks[i].Pass && result.Checks[i].Confidence < threshold {
        result.Checks[i].BelowThreshold = true
        allPass = false
    } else if result.Checks[i].Pass {
        passCount++
    }
}
result.Score = float64(passCount) / float64(len(result.Checks))
```

**Fix:** Count below-threshold checks as failures in the score denominator consistently. The current behavior is actually reasonable (below-threshold = failed, so score reflects effective pass rate), but the issue is that `Check.Pass` is still `true` while the check effectively failed. The score and `Check.Pass` disagree.

Better approach: don't recalculate score at all. Let Claude's score stand. Only override `result.Pass` (the top-level verdict). Callers who want the effective score can compute it from the checks themselves. This matches the design doc's intent: "Check.Pass = Claude's raw judgment, result.Pass = Claude's judgment AND confident enough."

```go
func applyConfidenceThreshold(result *EvalResult, threshold float64) {
    if threshold <= 0 {
        return
    }
    for i := range result.Checks {
        if result.Checks[i].Pass && result.Checks[i].Confidence < threshold {
            result.Checks[i].BelowThreshold = true
        }
    }
    // Recalculate only the top-level pass — any below-threshold check fails the eval.
    for _, c := range result.Checks {
        if !c.Pass || c.BelowThreshold {
            result.Pass = false
            return
        }
    }
}
```

Leave `result.Score` as Claude returned it.

---

## 8. Cache errors silently swallowed

**File:** `cache.go:58-79`

**Problem:** Both cache read errors (corrupt data) and cache write errors (marshal failure) are silently ignored. A broken cache implementation causes silent fallthrough to API calls with no logging.

**Fix:** When a logger or hook is configured on the session, log cache errors. The `cachedCaller` doesn't have access to the session though — it only wraps a `caller`.

Two options:
- (a) Pass the session's logger into `cachedCaller` at construction time
- (b) Use the `Event` hook — add an `Op: "cache"` event type

Option (b) is cleaner. Add the session reference to `cachedCaller`:

```go
type cachedCaller struct {
    inner   caller
    cache   Cache
    session *Session // for observability
}
```

Wire it in `buildSession`. Then in `call()`:
```go
if err := json.Unmarshal(data, &entry); err == nil {
    return entry.Raw, entry.Usage, nil
} else if c.session != nil {
    c.session.emit(Event{Op: "cache", Err: fmt.Errorf("cache read error: %w", err)})
}
```

Same for write errors.

---

## Testing

After all fixes, run:
```bash
go build ./...
go vet ./...
gofmt -l .
go tool golangci-lint run ./...
go test -count=1 -short ./...
ANTHROPIC_API_KEY=$(cat /Users/michaelhabib/dev/teams-sbx/.key) go test -count=1 -tags=e2e -timeout=5m ./...
```

Add unit tests for:
- ExtractResult[T].Usage populated
- Validator interface works with generic Extract[T]
- Fallback result has Fallback=true
- Timeout(-1) doesn't panic or apply timeout
- SENSE_SKIP with empty text returns skip result (not error)
- Nop() with options applies them
- Score unchanged by MinConfidence threshold
- Cache errors logged when logger set
