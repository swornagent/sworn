# Design TL;DR — S07-remaining-rescrape-cleanup

**Slice state at authoring:** `planned` → this doc gates entry to `design_review` (Rule 9: design review before code).

## User outcome (from spec.json)

Paging notifications report violations from the same source of truth as
everywhere else (`proof.json`), and the lint ledger's acceptance-check counts
and EARS classification come from `spec.json` instead of being independently
re-derived from `spec.md` text with their own, possibly-disagreeing
heuristics.

## Root cause (why the drift shipped)

Three more independent `spec.md`/`proof.md` scrapers, on top of the ones S02
and S04 already fixed in this release:

- `internal/account/notify.go`'s `ViolationsSummary(proofPath string, ...)`
  still does `os.ReadFile` + a numbered-list regex over `proof.md` — a third
  independent violations-parsing heuristic (S02's `blocked.go` and S04's
  `mcp/context.go` were the first two, both already migrated to
  `proof.json.not_delivered`). **Verified live**: S04's landed commits touch
  `internal/mcp/*` only; `internal/account/notify.go` is untouched and still
  reads `proof.md` (confirmed by direct read — see Investigation below). None
  of AC-01 is done yet on this base.
- `internal/ears/ears.go`'s `Validate` always classifies EARS keywords by
  running `Classify()` regexes over `spec.md` AC text, even for slices whose
  `spec.json.acceptance_criteria[].ears_keyword` was already computed once at
  write time by `internal/implement/spec_record.go`.
- `cmd/sworn/ledger.go`'s `countGates` counts `- [ ]` lines in `spec.md`
  instead of `len(spec.json.acceptance_criteria)`.

## Investigation (live repo state, this session)

- `grep -rn ViolationsSummary` → the only call site that invokes the function
  (not just sets the struct field) is `internal/run/slice.go:865`, in the
  `failed_verification` transition, passing `proofPath` (= `<sliceDir>/proof.md`).
  Confirms AC-01 scope is exactly one function + one call site.
- `internal/mcp/context.go`'s `readProofViolations(sliceDir string) string`
  (landed by S04) is the established in-release pattern for this exact
  problem: a small local unexported struct
  `struct{ NotDelivered []string `+"`"+`json:"not_delivered"`+"`"+` }`, no
  `proof.md` fallback. AC-01 mirrors this pattern rather than inventing a new
  one.
- `internal/spec.ReadRecord(sliceDir) (*Record, error)` (package doc: "the
  single reader for spec.json (spec-v1) records so that gates and verifiers
  consume one parser instead of each growing a bespoke scanner") already
  exists and already exposes `AC.EARSKeyword` and `Record.AcceptanceCriteria`.
  `internal/gate/trace.go` already consumes it (spec.md-first, spec.json as
  fallback only when spec.md is absent — an audit fix, not part of this
  slice's touchpoints). AC-02/AC-03 reuse this same reader rather than
  hand-rolling a second spec.json parser — but see DC-1 below for why the
  *source-preference order* deliberately differs from trace.go's.
- Distinct `ears_keyword` values seen across this release's real `spec.json`
  files: `"When"`, `"If"`, `"shall"` (case varies by writer — the
  markdown-scraping `spec_record.go` writer emits `"When"/"While"/"Where"/"If"/"Ubiquitous"`;
  a separate planner-authored path emits lowercase `"shall"` for the
  ubiquitous case). The schema (`spec-v1.json`) places no `enum` constraint on
  `ears_keyword` — it is a free-form string. The keyword→Pattern mapping in
  AC-02's design must be case-insensitive and default safely.
- `internal/implement/spec_record.go`'s `classifyEARSKeyword` (the writer) is
  a first-match sequential check (WHEN → WHILE → WHERE → IF+THEN → else
  Ubiquitous) — it can never emit a keyword representing `PatternComplex`
  (two-plus preconditions), unlike `ears.Classify`'s counting logic. Reading
  the stored keyword is therefore structurally coarser than re-deriving from
  prose for that one edge case. This is accepted, not fixed here (see DC-2).
- `cmd/sworn/ledger_test.go`'s `TestSync_GateCountFromSpec` fixture is
  `spec.md`-only (no `spec.json`) — it exercises exactly the legacy fallback
  path this design preserves, so it needs no change; a new spec.json-primary
  test is added alongside it.

## Approach

Same fix pattern as S02/S04/S06, applied to the three remaining touchpoints:
read the JSON field that already has the answer instead of re-deriving it
from prose, and keep a legacy (`spec.md`-only, pre-ADR-0009) fallback where a
consumer currently supports pre-migration slices.

### AC-by-AC design

- **AC-01 (`ViolationsSummary` reads `proof.json.not_delivered`):** change
  the function's first parameter from `proofPath string` (a `proof.md` path)
  to `sliceDir string` (the slice directory), matching
  `mcp/context.go readProofViolations`'s convention. Internally: read
  `filepath.Join(sliceDir, "proof.json")`, unmarshal into a local
  `struct{ NotDelivered []string `+"`"+`json:"not_delivered"`+"`"+` }`, and
  return the **first** entry (trimmed, truncated to 200 chars with `...`),
  preserving today's "one-line summary of the first violation" contract. No
  `proof.md` read remains in this function — the AC text says "instead of",
  not "in addition to". Preserve the existing fallback semantics for the
  no-data case: `violationCount > 0` → `"%d violation(s) found"`;
  `violationCount == 0` → `"verification failed"`; this fires whenever
  `proof.json` is missing, unreadable, or its `not_delivered` array is empty.
  Update the one call site in `internal/run/slice.go:865` from
  `account.ViolationsSummary(proofPath, ...)` to
  `account.ViolationsSummary(absSliceDir, ...)` — `absSliceDir` is already
  in scope at that point (defined at `slice.go:265`; a same-named but
  distinct local at `slice.go:615` is inside an unrelated inner block and
  does not shadow the one in scope at line 865). `proofPath` (the
  `.md` path) is untouched everywhere else in `slice.go` — it still backs
  `checkProofAbsent`, the first-pass gate's `ProofPath`, and the agentic
  verifier's prose payload, none of which are JSON *consumers* (the agentic
  verifier reads `proof.md` as prose *for an LLM*, not as a structured
  record) and none of which are in this slice's touchpoints.
- **AC-02 (`internal/ears` reads `ears_keyword` from `spec.json`):** add a
  pure mapping function `patternFromKeyword(keyword string) Pattern` in
  `ears.go`: case-insensitive match on `when`→`PatternEventDriven`,
  `while`→`PatternStateDriven`, `where`→`PatternOptionalFeature`,
  `if`→`PatternUnwanted`, `complex`→`PatternComplex` (forward-compatible,
  even though no current writer emits it), anything else (`shall`,
  `ubiquitous`, empty string, unrecognized) → `PatternUbiquitous` — mirroring
  the writer's own default-to-ubiquitous fallback. In `classifySpec`'s
  call site (or a new sibling function — see Files to touch), **prefer
  `spec.json` over `spec.md` whenever `spec.json` exists**, not just as a
  last-resort fallback (see DC-1 for why this order deliberately differs
  from `trace.go`'s). When `spec.json` is the source: iterate
  `rec.AcceptanceCriteria`, `Result.Text = ac.Text`,
  `Result.Pattern = patternFromKeyword(ac.EARSKeyword)`, `Result.Line` =
  1-based ordinal position in the array (there is no physical markdown line
  for a JSON record — documented as such, not a `spec.md` line number). Defensively
  still check `strings.HasPrefix(strings.ToUpper(ac.Text), "NOTE:")` → treat as
  `PatternNote` even though the current writer already filters `NOTE:` lines
  out of `spec.json` at write time (belt-and-braces, costs nothing, and
  matches the existing `spec.md` path's behaviour if a future writer ever
  changes). Fall back to today's `spec.md` text-classification path only when
  `spec.json` is absent or `len(rec.AcceptanceCriteria) == 0` (legacy,
  pre-ADR-0009 slices — the non-negotiable backward-compat constraint from
  intake.md).
- **AC-03 (`ledger.go` counts `len(spec.json.acceptance_criteria)`):**
  `countGates` tries `spec.ReadRecord(sliceDir)` first; if it returns a
  non-nil record with `len(rec.AcceptanceCriteria) > 0`, return that length.
  Otherwise fall back to the existing `- [ ]`-line scan of `spec.md`
  (unchanged code path, preserves `TestSync_GateCountFromSpec`). Reuses
  `internal/spec.ReadRecord` — no new parser.
- **AC-04 (JSON authoritative on disagreement, no historical reconciliation):**
  satisfied by construction — none of AC-01/02/03 touch any already-written
  `spec.md`/`spec.json`/`proof.md`/`proof.json` file; they only change which
  source future *reads* prefer. No migration/backfill script is written.
- **AC-05 (build + tests green):** `go build ./...` and
  `go test ./internal/account/... ./internal/ears/... ./cmd/sworn/...`.

## Files to touch (matches spec touchpoints exactly)

- `internal/account/notify.go` — `ViolationsSummary` signature + body
  rewired to `proof.json`; doc comment updated.
- `internal/account/notify_test.go` — `TestViolationsSummary_FromFile` and
  `TestViolationsSummary_Truncation` rewritten to write `proof.json` fixtures
  (`not_delivered: []string`) instead of `proof.md` prose; still exercise the
  same four cases (missing file, first-violation extraction, no-violations
  fallback, truncation).
- `internal/run/slice.go` — one-line call-site change at line ~865
  (`proofPath` → `absSliceDir`).
- `internal/run/run_test.go` — no change expected (its two `ViolationsSummary`
  assertions, `run_test.go:825` and `:887`, exercise the *end-to-end*
  `RunSlice` FAIL path via `NotifyEvent.ViolationsSummary`, not the function
  directly; they already drive through real `status.json`/`proof.json`
  fixtures per the test's own setup, so they should keep passing once the
  fixture — if it doesn't already carry `not_delivered` — is checked and
  topped up during implementation).
- `internal/ears/ears.go` — new `patternFromKeyword` mapping function;
  `Validate`/`classifySpec` (or a new `classifySpecJSON` sibling, implementer's
  call) gains the spec.json-preferred branch; import `internal/spec`.
- `internal/ears/ears_test.go` — new test(s) proving spec.json-sourced
  classification (including one proving JSON wins when `spec.md` text would
  classify differently, per AC-04); existing `spec.md`-only tests are
  untouched (no `spec.json` fixture written in a temp dir → same legacy path
  as today).
- `cmd/sworn/ledger.go` — `countGates` gains the `spec.ReadRecord`-first
  branch, falls back to the existing scanner.
- `cmd/sworn/ledger_test.go` — new `TestSync_GateCountFromSpecJSON` (writes a
  `spec.json` fixture with N acceptance_criteria, asserts `GateCount == N`);
  existing `TestSync_GateCountFromSpec` (spec.md-only fixture) is untouched.

No production files outside the six spec touchpoints (plus the one
call-site edit in `internal/run/slice.go`, which is not a listed touchpoint
but is required for AC-01's only caller to compile/behave correctly — flagged
here rather than silently expanding scope; `internal/run/slice.go` already
appears as a design-time discovery, not a hidden file).

## Design choices for reviewer

- **DC-1 (Type-2, local/reversible) — spec.json-preferred over spec.md-preferred,
  diverging from `trace.go`'s ordering.** `internal/gate/trace.go` prefers
  `spec.md` and only falls back to `spec.json` when `spec.md` is entirely
  absent, because it also runs *text-level* prose checks (the "see intake.md"
  reference check, vague-scope-AC check) that need the raw markdown body
  regardless of EARS classification — `spec.json` carries no markdown body to
  run those against. `internal/ears` has no such text-level check; its only
  job is per-AC EARS pattern classification, which `spec.json`'s
  `ears_keyword` already answers directly. AC-02's literal text ("instead of
  re-classifying it from `spec.md` prose text") and AC-04's "JSON
  authoritative on disagreement" both point at spec.json-first for `ears.go`
  specifically, even for a slice (like `S04-mcp-oracle-migration`, confirmed
  live to have both files) where `spec.md` also exists. Narrow — this is one
  function's source-of-truth order, reversible by flipping the branch order,
  and does not touch `trace.go` (out of this slice's touchpoints; the
  `gate/trace.go` audit fix already landed as a separate, prior change and is
  left alone).
- **DC-2 (Type-2, local/reversible) — accept the `PatternComplex` precision
  loss from reading the stored keyword instead of re-deriving from prose.**
  `spec_record.go`'s writer-side `classifyEARSKeyword` cannot produce a
  keyword representing two-or-more preconditions (it is a first-match
  sequential check, not a counting one), so a spec.json-sourced AC that would
  have classified as `PatternComplex` under `ears.Classify(text)` will
  instead classify per whichever single keyword the writer happened to pick.
  This is a real, name-able precision loss versus today's prose
  re-classification. Not fixed here: `spec_record.go` is not a touchpoint of
  this slice, and AC-04 explicitly scopes this slice to "which source future
  reads use," not to improving what the writer computes. Flagging for the
  Captain: if this precision loss is unacceptable, the correct owner is a
  follow-up slice on `spec_record.go`'s `classifyEARSKeyword`, not a scope
  change here.
- **DC-3 (Type-2, local/reversible) — `Result.Line` becomes an ordinal
  array-index for spec.json-sourced ACs, not a markdown line number.** The
  `Result`/`Violation` struct's `Line` field is documented as "1-based line
  number within the spec.md"; for a `spec.json`-sourced AC there is no
  markdown line to report. Using the 1-based position within
  `acceptance_criteria[]` keeps the field non-zero, deterministic, and
  reproducible (useful for a human reading a violation pointing at "AC #3"),
  at the cost of the doc comment's literal claim no longer being universally
  true post-change. The doc comment is updated to say so explicitly rather
  than left stale.
- **DC-4 (Type-2, local/reversible) — `ViolationsSummary` returns only the
  first `not_delivered` entry, not all of them joined.** Preserves today's
  "one-line summary" contract and the 200-char truncation budget (a paging
  notification field, not a full report). If a future consumer wants the
  full list, `proof.json.not_delivered` is already directly readable — no
  new API is invented to serve a need nobody has surfaced yet.

## Design-level risks

- `internal/run/run_test.go`'s two `ViolationsSummary`-adjacent assertions
  (`:825`, `:887`) depend on whatever `proof.json` fixture that test's setup
  already writes (or doesn't) for the FAIL-path scenario; if no
  `not_delivered` value is present in that fixture, the assertion at `:887`
  (`want "BLOCKED: spec missing required section"`) will need its fixture
  topped up with a matching `not_delivered` entry during implementation —
  called out here so it isn't discovered cold mid-implementation. **Verified
  live**: `TestRunSlice_BlockedNotifies`'s expectation comes from
  `lastVerdict.Rationale` at the BLOCKED-commit call site (`slice.go:824`,
  `summary := lastVerdict.Rationale`), not from `ViolationsSummary` — that
  call site, and the two other Notify call sites that hardcode their own
  summary strings (proof-absent at `slice.go:596`, first-pass gate at
  `slice.go:677`), never call `ViolationsSummary` at all and are untouched by
  this slice. Only `slice.go:865` (the FAIL/`failed_verification` path) calls
  the function this slice changes.
- `ears.go`'s `Distribution`/`TotalACs`/`TotalNotes` counters are consumed by
  `Print()` (human report) and by both CLI/MCP `lint` callers
  (`cmd/sworn/lint.go:93`, `internal/mcp/lint.go:65`) — neither caller reads
  `Result.Line` for anything beyond display, so DC-3's ordinal-vs-physical-line
  change carries no known downstream logic risk, only a display-semantics
  note.

## Traceability

| AC | Change | Test |
|----|--------|------|
| AC-01 | `ViolationsSummary(sliceDir, ...)` reads `proof.json.not_delivered` | `TestViolationsSummary_FromFile` (rewritten), `TestViolationsSummary_Truncation` (rewritten) |
| AC-02 | `ears.go` prefers `spec.json.ears_keyword` via `patternFromKeyword` | new `TestValidate_ReadsEARSKeywordFromSpecJSON` (+ a JSON-vs-md-disagreement case) |
| AC-03 | `ledger.go countGates` uses `len(spec.json.acceptance_criteria)` | new `TestSync_GateCountFromSpecJSON`; existing `TestSync_GateCountFromSpec` (spec.md-only fallback) unchanged |
| AC-04 | no historical reconciliation; JSON wins on disagreement | covered by AC-02's disagreement test case above |
| AC-05 | build + package tests green | `go build ./...`, `go test ./internal/account/... ./internal/ears/... ./cmd/sworn/...` |
