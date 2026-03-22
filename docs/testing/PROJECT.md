# AgentKit — Agent-Powered Test Assertions for Go

## What It Is

A Go testing library where AI agents validate AI agent output. One function, one pattern:

```go
func TestAgentProducesDoc(t *testing.T) {
    doc := runMyAgentE2E()

    agent.Assert(t, doc).
        Expect("covers all sections from the brief").
        Expect("includes actionable recommendations").
        Expect("does not hallucinate data sources").
        Run()
}
```

That's it. Under the hood: constructs a prompt, calls Claude with structured output (tool_use), gets back pass/fail per expectation, calls `t.Fatal()` if anything fails.

## Why It Exists

You're testing agents. Agents produce non-deterministic output. You can't `assert.Equal`. You can't `assert.Contains` because format varies. You need another agent to judge whether the output meets fuzzy, semantic requirements.

Today you'd either:
- Write brittle regex/string assertions that break on format changes
- Manually read the output every time (doesn't scale, can't CI)
- Skip assertions entirely and just check "did it error?" (misses quality)

AgentKit lets you write natural language assertions that an AI judge evaluates. Deterministic pass/fail from non-deterministic output.

## Design Principles

1. **One import, one pattern** — `agent.Assert(t, output).Expect("...").Run()`
2. **Go-native** — works with `go test`, `testing.T`, `testing.B`. No external tools.
3. **Opinionated defaults** — Sonnet, 3 retries, 30s timeout. You don't configure anything to start.
4. **Cost-conscious** — cached responses for deterministic re-runs. Quick mode for free smoke tests.
5. **No framework** — it's a library. Import it, use it, done.

## Document Index

| Doc | Description |
|-----|-------------|
| [design.md](design.md) | Technical design — how it works under the hood |
| [api.md](api.md) | Complete API reference |
| [implementation.md](implementation.md) | Build plan — what to build, in what order |
| [ideas.md](ideas.md) | Brainstorm — what else an opinionated agent API could do |
