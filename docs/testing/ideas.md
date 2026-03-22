# Ideas — What Else Can an Opinionated Agent API Do?

Beyond test assertions, here's what a `Do("instruction").Returns(&Struct{}).Run()` pattern unlocks. Each idea is a one-liner API that hides significant complexity.

---

## 1. Structured Extraction

Turn any blob of text into a typed struct. No parsing code.

```go
type Invoice struct {
    Vendor  string  `json:"vendor"`
    Amount  float64 `json:"amount"`
    DueDate string  `json:"due_date"`
}

invoice, err := agent.Extract("pull invoice details", pdfText).
    Into(&Invoice{}).
    Run()

db.Insert(invoice.Vendor, invoice.Amount, invoice.DueDate)
```

**What it hides:** Prompt engineering, JSON schema generation from struct tags, tool_use wiring, retry on malformed output, validation.

**Why it's valuable:** Every team has "parse this unstructured thing" tasks. Today it's regex, custom parsers, or manual extraction. This is one line.

---

## 2. Classification

Classify anything into a fixed set of categories.

```go
type Sentiment struct {
    Label      string  `json:"label" enum:"positive,negative,neutral"`
    Confidence float64 `json:"confidence"`
}

s, err := agent.Classify("sentiment", customerEmail).
    Into(&Sentiment{}).
    Run()

if s.Label == "negative" && s.Confidence > 0.8 {
    escalate(customerEmail)
}
```

**What it hides:** System prompt for classification, enum enforcement, calibration instructions.

**One step further — zero-struct shorthand:**
```go
label, err := agent.Classify("sentiment", text).
    Labels("positive", "negative", "neutral").
    Run()
// label == "negative"
```

---

## 3. Summarize

Summarize anything to a specific length/format.

```go
type Summary struct {
    Title     string   `json:"title"`
    Bullets   []string `json:"bullets" maxitems:"5"`
    TlDr      string   `json:"tldr" maxlen:"140"`
}

s, err := agent.Summarize(longDoc).
    Into(&Summary{}).
    Run()
```

**Even simpler:**
```go
tldr, err := agent.Summarize(longDoc).OneLine().Run()
// tldr == "Q3 revenue up 12%, APAC expansion delayed to Q4"
```

---

## 4. Diff / Change Analysis

Understand what changed between two versions of something.

```go
type Diff struct {
    Summary     string   `json:"summary"`
    Breaking    []string `json:"breaking_changes"`
    Additions   []string `json:"additions"`
    Removals    []string `json:"removals"`
    RiskLevel   string   `json:"risk_level" enum:"none,low,medium,high"`
}

diff, err := agent.Diff(oldSpec, newSpec).
    Context("this is our public API spec").
    Into(&Diff{}).
    Run()

if diff.RiskLevel == "high" || len(diff.Breaking) > 0 {
    requireApproval(diff)
}
```

---

## 5. Code Review

Review a diff/file for issues.

```go
type Review struct {
    Issues []Issue `json:"issues"`
    Score  float64 `json:"score"`
    Merge  bool    `json:"safe_to_merge"`
}

type Issue struct {
    Severity string `json:"severity" enum:"critical,warning,info"`
    Line     int    `json:"line"`
    Message  string `json:"message"`
    Fix      string `json:"fix,omitempty"`
}

review, err := agent.Review(gitDiff).
    Focus("security", "correctness").
    Context("payments service, handles PCI data").
    Into(&Review{}).
    Run()
```

---

## 6. Triage

Route/prioritize incoming items.

```go
type Triage struct {
    Priority string `json:"priority" enum:"P1,P2,P3,P4"`
    Team     string `json:"team"`
    Reason   string `json:"reason"`
    Urgent   bool   `json:"urgent"`
}

t, err := agent.Triage(alertPayload).
    Context("e-commerce platform, P1 = revenue impact").
    Into(&Triage{}).
    Run()
```

---

## 7. Decision

Get a structured recommendation with reasoning.

```go
type Decision struct {
    Choice    string   `json:"choice" enum:"approve,reject,escalate"`
    Reasoning string   `json:"reasoning"`
    Risks     []string `json:"risks"`
    Confidence float64 `json:"confidence"`
}

d, err := agent.Decide("should we deploy this to production?").
    Input(testResults).
    Input(coverageReport).
    Input(changeLog).
    Into(&Decision{}).
    Run()
```

---

## 8. Convert

Transform data from one format/language/schema to another.

```go
type GoCode struct {
    Code     string   `json:"code"`
    Compiles bool     `json:"compiles"`
    Imports  []string `json:"imports"`
}

result, err := agent.Convert(pythonCode).
    To("idiomatic Go").
    Into(&GoCode{}).
    Run()
```

---

## 9. Explain

Get a structured explanation of something complex.

```go
type Explanation struct {
    Summary    string   `json:"summary" maxlen:"200"`
    KeyPoints  []string `json:"key_points" maxitems:"5"`
    Complexity string   `json:"complexity" enum:"simple,moderate,complex"`
    Audience   string   `json:"audience" enum:"beginner,intermediate,expert"`
}

exp, err := agent.Explain(errorLog).
    For("a junior engineer").
    Into(&Explanation{}).
    Run()
```

---

## 10. Validate

Check if something meets a specification.

```go
type Validation struct {
    Valid      bool     `json:"valid"`
    Violations []string `json:"violations"`
    Warnings   []string `json:"warnings"`
    Score      float64  `json:"score"`
}

v, err := agent.Validate(apiResponse).
    Against(openAPISpec).
    Into(&Validation{}).
    Run()
```

---

## The Pattern

Every idea above is the same thing underneath:

```
verb(input) → prompt + struct schema → tool_use → typed result
```

The API is just **verbs**. Each verb is a thin wrapper that:
1. Picks the right system prompt for the task type
2. Takes your struct (or provides a sensible default)
3. Calls Claude with tool_use to enforce the schema
4. Returns a typed result

The opinionated part: **you don't write prompts, you don't configure schemas, you don't handle JSON parsing, you don't retry on failures.** You pick a verb, pass input, describe the shape of what you want back, and get it.

```go
// The universal pattern:
agent.Verb(input).Into(&Struct{}).Run()

// With options:
agent.Verb(input).
    Context("...").         // background for the judge
    Into(&Struct{}).        // what you want back
    Fast().                 // use Haiku (cheap)
    Run()
```

## Build Order

Start with `Assert`/`Eval` (the testing use case — that's the immediate need). Then `Extract` (most generally useful). Then `Classify` (simplest to implement). Each new verb is ~50 lines — a system prompt template and a builder method. The engine is the same.

## What Makes This Different from LangChain / Instructor / etc.

- **Go-native** — not a Python port
- **Verb-oriented** — you say what you want to do, not how to prompt
- **Opinionated** — one model, one approach, sensible defaults, minimal config
- **Struct-first** — your Go struct IS the contract. Tags add constraints. No external schema files.
- **Testing-first** — built for `go test`, not for chatbots
