# Implementation Plan

## Scope

A focused testing library. ~800-1,200 lines of Go.

```
agentkit/
├── agentkit.go        # Assert(), Eval(), Compare(), Configure()
├── assert.go          # AssertBuilder
├── eval.go            # EvalBuilder, EvalResult, Check
├── compare.go         # CompareBuilder, CompareResult
├── prompt.go          # Prompt templates, message construction
├── client.go          # Claude API client (tool_use)
├── cache.go           # Cache interface, FileCache, MemoryCache
├── quick.go           # Quick mode heuristics
├── config.go          # Config, env var loading
├── errors.go          # Error types, retry logic
├── go.mod
├── go.sum
└── testdata/
    └── cache/         # Cached API responses (committed)
```

---

## Phase 1: Core (1 week)

Get a working `Assert` that hits Claude and fails tests.

**Build order:**

1. **`config.go`** — Config struct, env var loading, global singleton
2. **`errors.go`** — Error types, retry with exponential backoff
3. **`client.go`** — Claude API client via Anthropic Go SDK. Constructs tool_use request with EvalResult schema. Extracts tool call arguments. Retry logic.
4. **`prompt.go`** — System prompt template. User message construction (format output + expectations). Input serialization (string, []byte, struct → text).
5. **`eval.go`** — EvalBuilder. Expect(), Context(), Judge(). EvalResult, Check types. Calls client, unmarshals result.
6. **`assert.go`** — AssertBuilder. Thin wrapper around EvalBuilder. Calls t.Fatal() with formatted output on failure.
7. **`agentkit.go`** — Package-level Assert(), Eval(), Configure() entry points.

**Tests:**
- Unit: prompt construction, result formatting, config loading
- Integration (gated): real Claude call with a known input, assert on EvalResult structure

**Done when:**
```go
func TestSmoke(t *testing.T) {
    agent.Assert(t, "hello world").
        Expect("contains a greeting").
        Run()
}
```
This runs, hits Claude, and passes.

---

## Phase 2: Cache + Quick (3-4 days)

Make it practical for CI.

**Build order:**

1. **`cache.go`** — Cache interface. Content-addressed key generation (SHA-256 of model + prompt + message). FileCache (JSON on disk). MemoryCache (in-memory map).
2. **Wire cache into client** — Check cache before API call, store after.
3. **`quick.go`** — Quick mode. Pattern parser ("contains X" → strings.Contains, etc.). ErrQuickUnsupported for unrecognized patterns.
4. **Wire Quick into builders** — `.Quick()` method on Assert/Eval.
5. **`AGENTKIT_SKIP`** — Skip mode for offline dev.

**Tests:**
- Cache hit/miss, key determinism, file I/O
- Quick mode: every supported pattern, unsupported pattern error
- Skip mode: assert is no-op when AGENTKIT_SKIP=1

**Done when:**
- Second test run is instant (cache hit)
- `testdata/cache/` has JSON files that can be committed
- Quick mode works for simple string patterns

---

## Phase 3: Compare + Consensus (3-4 days)

Round out the API.

**Build order:**

1. **`compare.go`** — CompareBuilder. Criteria(). Judge(). Prompt template for A/B comparison. CompareResult.
2. **Consensus mode** — `.Consensus(n)` on EvalBuilder/AssertBuilder. Run N judges concurrently with errgroup. Aggregate votes.
3. **Token/cost tracking** — Add TokensUsed and Duration to EvalResult. Optional Tracker for cumulative stats.

**Tests:**
- Compare: known-winner test case
- Consensus: mock 3 judges with 2/3 agreement
- Tracker: accumulation across calls

**Done when:**
- Compare works with real API
- Consensus runs 3 judges concurrently
- Test output shows token usage

---

## Phase 4: Polish + Ship (2-3 days)

- README with real examples
- godoc on every exported symbol
- `go vet` + `golangci-lint` clean
- CI workflow (GitHub Actions)
- `go.mod` published, `go get` works
- One real-world example: assert on Cortex e2e test output

---

## Total: ~3 weeks

| Phase | Duration | What |
|-------|----------|------|
| 1: Core | 1 week | Assert, Eval, Claude client, prompts |
| 2: Cache + Quick | 3-4 days | Caching, quick mode, skip mode |
| 3: Compare + Consensus | 3-4 days | A/B comparison, multi-judge |
| 4: Polish | 2-3 days | Docs, lint, CI, ship |

---

## Dependencies

```
go.mod:
    github.com/anthropics/anthropic-sdk-go  (Claude API)
```

That's it. One dependency beyond stdlib. The Anthropic Go SDK handles HTTP, auth, streaming, retries at the API level. AgentKit handles structured output, prompt construction, caching, and test integration.
