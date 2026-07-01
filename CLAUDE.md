# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Sense is a Go library that makes sense of non-deterministic text via the Anthropic API: **Extract** (unstructured text → typed Go struct) and **Judge** (evaluate output against natural-language expectations — `Assert`/`Require` in tests, `Eval`/`Compare` programmatically). It is a single flat package (`package sense`, module `github.com/itsHabib/sense`); there is no `cmd/` or sub-packages.

## Commands

```bash
go test ./...                                   # unit tests — use a mock caller, no API, no key needed
go test -run '^TestCachedCaller_HitSkipsInner$' .  # a single test (package is at repo root, so `.`)
go test -tags=e2e -v ./...                      # e2e tests — hit the real API and COST money (~$0.10–0.15/run)
go test -tags=e2e -run TestEval_FailsOnPlainText -v .  # a single e2e test
SENSE_SKIP=1 go test ./...                      # offline — every sense call becomes a passing no-op
go tool golangci-lint run                       # lint (golangci-lint v2 is pinned as a go.mod `tool` dependency)
go build ./...
```

- Unit tests inject a mock `caller` (see `unit_test.go`) and never touch the network. E2e tests live behind the `//go:build e2e` tag in `e2e_test.go`.
- Requires Go 1.25+ (uses `for range int`, the `tool` directive, generic type inference).
- `ANTHROPIC_API_KEY` is only needed for e2e runs and real usage, not for unit tests.

<!-- BEGIN dev-workbench (managed by /dev-workbench skill — re-run to refresh; hand-edits inside this block will be overwritten) -->
## Dev workbench

Several MCP servers + skills are available in any Claude session on this machine — the dev-workflow infrastructure built across the portfolio: dossier (project memory), ship (workflow execution), huddle (multi-seat coordination), playwright (browser automation), plus the `/work-driver` family and the `/worktree-*` skills. When the signal matches, **just call the verb**. Don't ask permission. Stuck on a *knowledge* question about another portfolio repo — how it behaves, its conventions, what's in flight there — `/consult` its steward instead of asking the operator; only *authority* questions (direction, spend, irreversible calls) go to the operator.

### dossier — project memory

Long-term home for what's planned, in-flight, and shipped across the portfolio. Projects → phases (design docs) → tasks → artifacts (PRs / commits / files). Markdown-on-disk corpus; the on-disk format IS the source of truth.

**Use proactively for:**

- *"What's the state of `<project>`?"* → `mcp__dossier__project_get { slug }`, then `mcp__dossier__phase_list` + `mcp__dossier__task_list { project, status: ["in_progress"] }`.
- *"I'm starting `<new chunk of work>`."* → `mcp__dossier__phase_add { project, slug, title, body }`.
- *"I need to do X"* / discrete actionable surface → `mcp__dossier__task_create { project, phase?, slug, title, body }` (status defaults to `todo`).
- User picks up a task → `mcp__dossier__task_claim { id, actor: "human:michael" }`. Re-claim by same actor is a no-op.
- Progress on a task → `mcp__dossier__task_update { id, status?, note?, ... }`. Append notes liberally — the corpus IS the working log.
- Open / merged PR → `mcp__dossier__artifact_link { project, task?, kind: "pr"|"commit", ref, label }` without being asked.
- *"Done with task X."* → `mcp__dossier__task_complete { id, note? }`.

**Don't use for:**

- Code-level work (write the code first; *then* `artifact_link` the PR).
- Anything that only matters within this session's scratch context.

### ship — workflow execution

Hands a task doc to a coding agent (cursor), persists what happened, lets you inspect / cancel / replay the run. Owns nothing about the workspace (the `/worktree-*` skills handle that) or the planning (dossier's job). Async — every `ship` returns `{ workflowRunId, status: "running" }` immediately; poll `get_workflow_run` for the terminal state.

**Use proactively for:**

- *"Ship `<task doc>` against `<worktree>`."* → `mcp__ship__ship { docPath, workdir, repo, branch, runtime: "local" }`.
- *"Ship `<task doc>` on cursor cloud (no local worktree)."* → `mcp__ship__ship { docPath, runtime: "cloud", cloud: { repos: [{ name }], env: { type: "cloud" }, autoCreatePR: true } }`. Cloud runs open the PR themselves via `autoCreatePR`; never set `skipReviewerRequest: true`.
- *"What ran on `<repo>` recently?"* / *"What's still in flight?"* → `mcp__ship__list_workflow_runs { repo?, status?, limit? }`.
- *"What did `<wf id>` do?"* → `mcp__ship__get_workflow_run { workflowRunId }` (cloud runs include a `watchUrl` live dashboard link).
- In-flight run needs to stop → `mcp__ship__cancel_workflow_run { workflowRunId }` (idempotent on terminal rows).
- Inspect cloud outputs → `mcp__ship__list_artifacts { workflowRunId }`, then `mcp__ship__download_artifact { workflowRunId, path }`.

**Don't use for:**

- Creating the worktree (use `/worktree-add`).
- Writing the task doc (a normal file edit inside the worktree).

### huddle — multi-seat coordination

Go MCP server that opens a Slack channel per "huddle," issues per-seat keys (each key = an identity), and lets agents post + read with automatic attribution; the operator is the implicit orchestrator. Reach for it only when more than one agent/seat needs to coordinate in a shared room.

**Use proactively for:**

- *"open a room for `<multi-agent effort>`"* → `mcp__huddle__huddle_create`.
- *"who else is in here?"* → `mcp__huddle__huddle_who_else`.
- *"post an update to the room"* → `mcp__huddle__huddle_post`; *"what's been said?"* → `mcp__huddle__huddle_read`.
- *"list active huddles"* → `mcp__huddle__huddle_list`; close one out → `mcp__huddle__huddle_close`.

**Don't use for:**

- Solo sessions — there's no one to coordinate with.
- Durable project state (that's dossier) or run logs (that's ship).

### playwright — browser automation

Drives a real browser for anything that needs a rendered page: verifying a UI change, reproducing a browser-only bug, scraping. Snapshot-first — `browser_snapshot` returns an accessibility tree whose refs you act on, rather than clicking pixels.

**Use proactively for:**

- *"open `<url>` and check `<thing>`"* → `mcp__plugin_playwright_playwright__browser_navigate { url }`, then `browser_snapshot`.
- *"click / type / fill the form"* → `browser_click`, `browser_type`, `browser_fill_form` (act on refs from the snapshot).
- *"screenshot it"* → `browser_take_screenshot`.
- *"what did the page request / log?"* → `browser_network_requests`, `browser_console_messages`.

**Don't use for:**

- Pure HTTP/JSON checks — use `curl` or a small script; a browser is overkill.

### `/work-driver` — drive agent-led impl end-to-end

Drives one or N parallel streams from task doc to merged PR: pre-flight worktrees (local) or skip (cloud), fan out via `mcp__ship__ship`, poll terminal states, verify the commit (local) / trust cloud status, open PRs (local manually / cloud via `autoCreatePR`), coordinate review cycles, merge in dependency order. The conductor for the whole workbench.

**Triggers:** "drive these tasks to merge", "ship these N tasks in parallel", "run the work driver", explicit `/work-driver`.
**Pair with:** `/work-driver-prep` to generate the specs first; `/shipped` after to recap what landed.

### `/work-driver-prep` — specs + batched plan from dossier tasks

Resolves task IDs or a phase slug, writes one spec doc per task, detects file-overlap conflicts, groups into parallel-safe batches, and emits ready `/work-driver` commands. The planning front-end that feeds `/work-driver`.

**Triggers:** "prep these tasks for the driver", "draft specs for phase `<slug>`", explicit `/work-driver-prep`.
**Pair with:** `/work-driver` (runs the batches it emits); `/work-driver-seed` or `/tdd` upstream when the tasks don't exist yet.

### `/shipped` — retrospective recap of landed work

After a chunk lands: PRs merged + weighted-LOC, dossier task closures, chips filed, friction-log delta, what changed about main, what's open, next moves. Read-only — it reports, it doesn't act.

**Triggers:** "what just shipped", "what merged today", "post-run summary", explicit `/shipped`.
**Pair with:** the natural follow-up to a `/work-driver` run.

### `/status` — tight in-flight status update

Four sections — What happened / What's next / What I recommend / What I need from you — 1–3 sentences each. The mid-session counterpart to `/shipped` (in-flight vs. retrospective).

**Triggers:** "give me an update", "where are we", "sitrep", "recap", explicit `/status`.
**Pair with:** `/shipped` once the work actually lands.

### `/consult` — summon a sibling repo's steward

Summons a sibling repo's steward (an ephemeral subagent scoped to that repo) for a same-turn answer; knowledge questions go to a peer, authority questions to the operator. Read-only — it answers, it doesn't act.

**Triggers:** "ask the `<repo>` agent", "what does the `<repo>` steward think", stuck on how another portfolio repo behaves, explicit `/consult <repo> "<question>"`.
**Pair with:** the stuck-path escalation before pinging the operator mid-task or mid-`/work-driver`.

### `/worktree-*` — manage secondary git worktrees

Thin skill family over plain `git worktree`. Use these instead of reaching for an MCP — they cover the verbs that mattered (add, list, remove, transfer, where) without an external state store.

- **`/worktree-add`** — *"spin up a worktree for `<ticket>`"* → creates `.claude/worktrees/<branch>/`, copies untracked CLAUDE.md if present
- **`/worktree-list`** — *"what worktrees do I have"* → branch, dirty state, optional PR/CI from `gh`
- **`/worktree-remove`** — *"clean up the worktree"* → dirty-state aware (commit-WIP / stash / discard)
- **`/worktree-transfer`** — *"bring this work over to main"* → removes secondary, checks out branch in root
- **`/worktree-where`** — *"where am I"* → which worktree, branch, and cwd this session is pointing at

### The loop

```
dossier task ──▶ /worktree-add ──▶ ship run ──▶ PR ──▶ review cycle ──▶ merge
   (plan)          (isolate)       (execute)         (codex+claude+cursor)  │
     ▲                                                                      ▼
     └─────────── /shipped recap ◀── dossier task_complete + /worktree-remove
```

`/work-driver` orchestrates the whole row; `/work-driver-prep` builds the task specs that seed it.

### Why this shape

Each layer is independently swappable. dossier could be Linear; the `/worktree-*` skills could be hand-rolled `git worktree` calls; ship could be a different agent runner; huddle could be any chat transport. The seams are deliberate — planning (dossier), isolation (worktree-\*), execution (ship), and coordination (huddle) don't reach into each other, so substituting one doesn't ripple into the rest; `/consult` owns the stuck path (peer knowledge before operator attention). The skills are compositions over these mechanisms, not new state stores.
<!-- END dev-workbench -->

## Architecture

### The `caller` seam is the center of everything

Every operation — eval, compare, extract, extract-slice, parallel extract — ultimately calls `session.client.call(ctx, *callRequest) (json.RawMessage, *Usage, error)`. That one-method `caller` interface (`client.go`) is the seam the whole library pivots on. Four implementations, composed in `buildSession` (`option.go`):

- `claudeClient` (`client.go`) — the real API call. Sends exactly one tool with `ToolChoice` forcing it, then pulls the `ToolUseBlock`'s raw JSON input back out. Retries rate-limits / 5xx with exponential backoff.
- `batchCaller` + `batcher` (`batch.go`) — routes calls through the Anthropic Batch API for 50% cost. A single background goroutine owns the pending slice (no mutex); callers block on a per-request response channel. **Requires `Session.Close()`** to flush.
- `cachedCaller` (`cache.go`) — a **decorator** that wraps any other caller; content-addressed key over model+prompts+schema. `buildSession` stacks it on top of the base caller when a cache is configured.
- `nopCaller` (`nop.go`) — returns `{}`; used by `sense.Nop()`.

When adding a feature, ask "does this belong behind the `caller` seam (a new transport/decorator) or in front of it (a new builder)?" — that decision keeps the rest of the code untouched.

### Forced tool_use is the whole trick

There is no prompt-for-JSON and no output parsing. Each `callRequest` carries a `toolName` + `toolSchema`; the request forces that single tool via `tool_choice`, so the model's output shape is enforced server-side. Sense just unmarshals the tool-call arguments into your Go type. The eval/compare tool schemas are hand-written constants in `client.go` (`evalToolSchema`, `compareToolSchema`); extract schemas are generated by reflection (below).

### Builders converge on the caller

Each operation is a chainable builder with a terminal method:

| Builder | Created by | Terminal | File |
|---|---|---|---|
| `EvalBuilder` | `Eval(output)` | `Judge()` | `eval.go` |
| `AssertBuilder` | `Assert`/`Require(t, output)` | `Run()` (wraps an `EvalBuilder`, calls `t.Error`/`t.Fatal`) | `assert.go` |
| `CompareBuilder` | `Compare(a, b)` | `Judge()` | `compare.go` |
| `ExtractBuilder[T]` | `Extract[T](text)` (generic, returns `ExtractResult[T]`) | `Run()` | `extract.go` |
| `ExtractIntoBuilder` | `s.Extract(text, &dst)` (json.Unmarshal-style) | `Run()` | `extractor.go` |
| `ExtractSliceBuilder[T]` | `ExtractSlice[T](text)` | `Run()` | `extract_slice.go` |

Chainable options (`Context`, `Model`, `Timeout`, `Validate`, `Fallback`, `MinConfidence`) just set fields; all the real work and the single `caller` call happen in the terminal. To add an operation, mirror this shape and reuse the prompt/timeout/usage helpers in `prompt.go` and `observe.go`.

### Session and the three entry tiers

`Session` (`observe.go`) holds the `caller`, model, timeout/retries, optional cache + batcher, base context, min-confidence, logger/hook, and atomic usage counters. Three ways in, increasing control:

1. **Package-level funcs** (`default.go`) — `sense.Assert`, `sense.Eval`, `sense.Extract[T]`, etc. use a lazily-created default `Session` (`sync.Once` singleton). Zero config.
2. **`ForTest(t, opts…)`** (`for_test_helper.go`) — a session bound to the test via `t.Cleanup` (auto-`Close`, logs a usage summary).
3. **`New(opts…)`** — full functional-options config. `Nop(opts…)` builds a session whose caller is the no-op.

Two interfaces, `Evaluator` and `Extractor` (`evaluator.go`, `extractor.go`), let callers depend on a behavior rather than `*Session` for testability; `*Session` satisfies both (asserted with `var _ Evaluator = (*Session)(nil)`).

### Config resolution and the sentinel pattern

Options accumulate into a `sessionConfig`; `applyDefaults` fills gaps (model `claude-sonnet-4-6`, timeout 30s, retries 3, key from `$ANTHROPIC_API_KEY`); `buildSession` constructs the caller stack. Note the **`timeoutSet`/`maxRetriesSet` sentinel bools** — they distinguish "caller never set it" from "caller set it to 0", so `WithTimeout(0)`/`WithRetries(-1)` can mean "disable" rather than "use default". Preserve this when adding zero-meaningful options.

Model precedence: per-call `.Model()` > `$SENSE_MODEL` > session model (see `getModel` + each builder).

### Schema generation (extract only)

`extract_schema.go` reflects a struct into an `anthropic.ToolInputSchemaParam`, cached in a `sync.Map` keyed by `reflect.Type`. `json` tags name fields, `sense:"…"` tags describe them; **value fields are required, pointer fields are optional**. Two entry points mirror the two extract APIs: `schemaFor[T]()` (generic) and `schemaForValue(dest)` (runtime, for `s.Extract(text, &dst)`).

### Validation, skip mode, usage, errors

- **Validation** runs after unmarshal, two ways that compose: a `.Validate(fn)` closure, and the `Validator` interface (`Validate() error`) auto-detected on the destination type. Closure runs first.
- **`SENSE_SKIP=1`** is checked at the top of every terminal method — it short-circuits to a passing/zero result before any setup, so suites run with no key and no cost.
- **Usage** is tracked with atomic counters on the session (`recordUsage` after every call); `Usage()` returns a `SessionUsage` snapshot with an estimated cost from the `modelPricing` table in `observe.go`. Update that table when model pricing changes.
- **Errors**: sentinel values (`ErrRateLimit`, `ErrTimeout`, `ErrNoToolCall`, `ErrNoExpectations`, …) plus an `*Error` wrapper with `Op`/`Message`/`Err` and `Unwrap` (`errors.go`).
- **Confidence threshold**: `applyConfidenceThreshold` (`eval.go`) demotes low-confidence passes to `BelowThreshold` and fails the eval; it deliberately leaves `Score` as Claude reported it.

## Conventions

- **Project state** lives in `PROJECT.state.yaml` (per-track/phase status), with design notes under `docs/`. `What's Next` in the README and the `eval-quality`/`cost-safety`/`ci-integration` tracks are the live roadmap — `cache.go` is wired but disk/prompt caching and cost budgets are not yet built.
- **House style is Dave Cheney's** and is enforced, not just aspirational — see `.golangci.yml` (revive `indent-error-flow`/`superfluous-else`, `nestif`, `funlen` 80 lines, `gocyclo` 15). Keep the happy path unindented, prefer early returns over `else`, keep functions short. (See **Engineering principles** below for the *why*.)
- `docs/` and `testdata/` are excluded from lint.

<!-- BEGIN eng-philo (managed by /eng-philo — re-run to refresh; hand-edits inside this block will be overwritten) -->
## Engineering principles

How code is written here — Dave Cheney lineage ([Practical Go](https://dave.cheney.net/practical-go)): simplicity, clarity, line-of-sight. Apply on every change; the lint below catches the slips. These are the *why* above the lint list in `## Conventions`.

1. **No `else` — line-of-sight.** Handle errors and edge cases with early returns and guard clauses; keep the happy path un-indented, flowing down the left margin. Reaching for `else` → return early instead.
2. **Shallow nesting — ≤2 levels per scope.** A `for` + an `if` is the ceiling in one scope. The budget is per-scope, not per-function — a closure is its own scope. Deeper in one scope → extract a function.
3. **Policy vs mechanism.** Separate the decisions (policy: validation, state machines, business rules) from the plumbing (mechanism: persistence, transport, I/O). Mechanism is dumb and swappable; policy lives in a layer above it. Never let policy leak into a mechanism layer.
4. **Composition of single-responsibility layers.** Each layer owns ~one responsibility; the app is a *composition* of them; any piece is swappable without rippling into the others. Dependencies flow one direction.
5. **Small, sharp APIs.** Export the least callers need. Intention-revealing names. Accept the narrowest input, return concrete types. Make the zero value useful.
6. **Errors are values; simplicity over cleverness.** Handle or propagate errors explicitly — never swallow. Readable > clever > short. A little copying beats a premature abstraction or dependency.

### Go idioms + enforcement

- **Accept interfaces, return structs.** Take the narrowest interface a function needs (the `Evaluator` / `Extractor` seams, the internal one-method `caller`); hand back concrete types (`*Session`, `*EvalResult`, `*ExtractResult[T]`).
- **Small interfaces.** 1–2 methods. `caller` is one method; `Validator` is one. Keep them that way — a new transport implements `caller`, nothing more.
- **Errors lowercase, wrapped with `%w`.** Sentinel `Err*` values plus the `*Error` wrapper with `Unwrap` (`errors.go`); never `fmt.Errorf("Failed to …")`.
- **Early-return / line-of-sight.** The happy path is not indented; guard and return. `shouldSkip()` / empty-input checks sit at the top of every terminal method.
- *Enforce:* `go tool golangci-lint run` — `revive` (`indent-error-flow`, `superfluous-else`), `nestif` (min-complexity 4), `gocyclo` (15), `gocognit` (20), `funlen` (80 lines). Config in [.golangci.yml](.golangci.yml).
<!-- END eng-philo -->
