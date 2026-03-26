# API Simplification — Session Ceremony Problem

## The Problem

Every user's first interaction with sense requires three lines before they do anything useful:

```go
s := sense.NewSession(sense.Config{})
defer s.Close()
s.Assert(t, output).Expect("...").Run()
```

`Close()` is a no-op unless batching is enabled (off by default). `Config{}` is empty for the default case. A Reddit commenter asked "why is there a concept of session?" — and they're right that it's not obvious.

Session exists because it holds: API client (key + retries), model/timeout config, atomic usage counters, and optionally the batch processor. It accumulates state across calls, so it's more than config. But for the common case — "judge this output with defaults" — it's pure ceremony.

## What Session Actually Does

- Holds the `caller` (API client or batch client)
- Stores model, timeout, maxRetries
- Tracks token usage via atomic counters (`Usage()`)
- Manages batch lifecycle (`Close()` flushes pending batch requests)

Only batching and usage tracking require statefulness. Model/timeout/retries are just config.

## Options

### 1. Package-level defaults (lazy session)

```go
sense.Assert(t, output).Expect("...").Run()
sense.Eval(output).Expect("...").Judge()

var m MountError
sense.ExtractInto("device /dev/sdf ...", &m).Run()

fmt.Println(sense.Usage()) // usage on the default session
```

A `sync.Once`-initialized session using env defaults behind the scenes.

**Pros:**
- Zero boilerplate for the 80% case
- README examples become one-liners
- Lowest barrier to adoption

**Cons:**
- Hidden global state — harder to reason about in concurrent tests
- No way to Close() it (batch users can't use this path)
- `Usage()` on a global you didn't create feels magic
- Two ways to do everything (package-level + Session methods)

### 2. Shorter constructor with functional options

```go
s := sense.New()
s := sense.New(sense.WithModel("claude-haiku-4-5-20251001"))
s := sense.New(sense.WithBatch(50, 2*time.Second))
defer s.Close()
```

Replace `NewSession(Config{})` with `New(...opts)`. Session stays explicit.

**Pros:**
- Cleaner than `NewSession(Config{})`
- Session is still explicit — no hidden state
- Functional options are idiomatic Go (Dave Cheney pattern)

**Cons:**
- Still two lines (New + defer Close)
- Breaking change to the constructor
- Doesn't solve the "why session?" question

### 3. Test helper with t.Cleanup

```go
s := sense.ForTest(t)                                    // defaults
s := sense.ForTest(t, sense.WithModel("claude-haiku-4-5-20251001"))  // custom
// t.Cleanup(s.Close) registered automatically
```

**Pros:**
- One line in tests, no defer needed
- Cleanup scoped to the test automatically
- Could print usage summary in test logs on cleanup

**Cons:**
- Only helps test code, not production Extract users
- Another constructor to learn

### 4. Config-as-receiver (drop Session entirely)

```go
c := sense.Config{Model: "claude-haiku-4-5-20251001"}
c.Assert(t, output).Expect("...").Run()
```

**Pros:**
- No lifecycle management
- "Why session?" question goes away

**Cons:**
- Config becomes mutable (usage counters) — misleading name
- Batch lifecycle still needs somewhere to live
- Big refactor, breaks everything

### 5. Three-tier API (recommended exploration)

Combine options 1 + 2 + 3 for progressive disclosure:

```go
// Tier 1: Zero-config — just works
sense.Assert(t, output).Expect("...").Run()

// Tier 2: Test suite — auto-cleanup, usage tracking
s := sense.ForTest(t)
s.Assert(t, output).Expect("...").Run()
t.Log(s.Usage())

// Tier 3: Power users — full control, batching
s := sense.New(sense.WithBatch(50, 2*time.Second))
defer s.Close()
```

Each tier adds one line of setup. Users start at tier 1 and graduate when they need more control.

**Pros:**
- Easiest possible onboarding (one function call)
- Progressive complexity — you only learn Session when you need it
- Test helper gets cleanup for free

**Cons:**
- Three ways to do the same thing
- Package-level functions are hidden global state
- More API surface to maintain and document

## Implementation Notes

- Package-level functions are thin: `func Assert(t testing.TB, output any) *AssertBuilder { return defaultSession().Assert(t, output) }`
- Default session: `var once sync.Once; var def *Session` — initialized on first call
- `ForTest(t)` is: `s := New(opts...); t.Cleanup(s.Close); return s`
- `New()` replaces `NewSession(Config{})` — can keep `NewSession` as deprecated alias
- None of this changes the `Evaluator`/`Extractor` interfaces — they still work

## Open Questions

- Should the default session be configurable? e.g. `sense.SetDefault(sense.New(sense.WithModel(...)))`
- Should `ForTest` print a usage summary to the test log on cleanup?
- Is the three-tier API too much surface area for a small SDK?
- Does the package-level path need its own `Usage()` function, or is that a footgun?

