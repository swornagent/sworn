# Proof Bundle: `S04-inprocess-oai-driver`

Rendered from `proof.json` (proof-v1). Generated from live repo state, 2026-07-10.

## Scope

Wrap the existing `internal/agent.Run` tool loop and the `model.NewClient`-resolved
OAI/Responses clients as ONE hardened in-process driver behind the
`driver.Driver` contract, with the verifier role running the tool loop for
investigation before emitting its verdict via exactly one `ChatStructured`
call (closes the sworn#55 gap for in-process dispatch).

## Files changed

```
$ git diff --name-only 6ce9d30526cd8eb1911fa8916ad8df8a8a117fed HEAD
docs/release/2026-06-28-driver-contract/S04-inprocess-oai-driver/journal.md
docs/release/2026-06-28-driver-contract/S04-inprocess-oai-driver/status.json
internal/driver/inprocess/inprocess.go
internal/driver/inprocess/inprocess_test.go
internal/driver/inprocess/inprocess_verify.go
```

(Plus, after implementation: `proof.json`, `proof.md`, and the `status.json`
state flip to `implemented` — the proof/state artefacts themselves.)

## Test results

```
$ go build ./...
(exit 0, no output)

$ go vet ./internal/driver/...
(exit 0, no output)

$ go test -timeout 120s ./internal/driver/...
ok  	github.com/swornagent/sworn/internal/driver	1.675s
ok  	github.com/swornagent/sworn/internal/driver/inprocess	0.153s

$ go test -timeout 300s ./...
(all packages ok, zero failures — full-suite output captured in session; the
merge gate owns full-suite re-verification)
```

`gofmt -l internal/driver/inprocess/` clean; newline-corruption grep over the
new files: zero hits.

## First-pass verification gate

```
$ sworn verify --spec docs/release/2026-06-28-driver-contract/S04-inprocess-oai-driver/spec.json \
    --diff <start_commit..HEAD> \
    --proof docs/release/2026-06-28-driver-contract/S04-inprocess-oai-driver/proof.json \
    --verifier-model deepseek/deepseek-chat
{ "verdict": "PASS" }
```

`sworn llm-check -type ac-satisfaction` exits on `read spec.md: no such file`
for this spec-v1 (`spec.json`) slice — the known format-lag false-negative
class. Manual AC-to-test cross-check performed instead (every AC has a named
test below).

## Reachability artefact

`cli-run` — `go test ./internal/driver/inprocess/ -run TestInprocessImplementer -v`:
`InProcess.Dispatch` drives the REAL `internal/agent.Run` loop against an
httptest `/chat/completions` server scripting a tool-call turn then a
terminal turn — the `bash` tool actually executes (`echo hi`) inside a real
`git init` worktree gated by `AssertWorktree` (Rule 11), and the `Result`
carries the loop's final text plus accumulated `InputTokens=24` /
`OutputTokens=12`, `DurationMS>0`, provider-confirmed `ModelID`,
`CostSource=estimated`. `TestInprocessVerifier` exercises the full verifier
path end-to-end through the same integration point: investigation loop (tool
executed), then exactly ONE `ChatStructured` (`response_format`) call whose
request body demonstrably carries the replayed transcript (`role:tool`
message asserted in the raw bytes), verdict returned unmodified in
`StructuredJSON`. The engine does not call `Dispatch` until S06 — the Driver
contract is the integration point that owns this affordance (ADR-0012);
real-provider integration proof is S10's conformance SIT.

## Delivered

- **AC-01** — implementer dispatch runs the multi-turn tool loop rooted at
  `WorktreeRoot` after the S01 `AssertWorktree` guard; `Status=ok`,
  `ResultText` = loop final text, tokens accumulated across turns,
  `DurationMS` populated. Evidence: `inprocess_test.go:TestInprocessImplementer`.
- **AC-02** — verifier dispatch: tool loop for investigation, then exactly
  one `ChatStructured` call over the accumulated transcript against
  `DispatchInput.VerdictSchema`, returned unmodified in
  `Result.StructuredJSON`. Evidence: `inprocess_test.go:TestInprocessVerifier`
  (asserts `structuredRequestCount==1` + transcript in the verdict request).
- **AC-03** (narrowed to the `oai-inprocess` chat identity, captain-proceed.md
  pin 3) — tool-only assistant turns still serialize a present `content`
  field, asserted on raw request bytes. Evidence:
  `inprocess_test.go:TestInprocessContentAlwaysPresent`.
- **AC-04** — max-turns → `ErrKind=transient` (retryable, `errors.Is`
  `agent.ErrMaxTurns`); structured-emission failure → `ErrKind=protocol`;
  no panic, no fabricated verdict. Evidence:
  `inprocess_test.go:TestInprocessMaxTurnsTransient`,
  `TestInprocessVerdictEmissionProtocol`.
- **AC-05** — no edits to `internal/agent` or `internal/model`; diff touches
  only `internal/driver/` files + slice docs; `go test ./internal/driver/...`
  passes including S01's untouched `TestNoWireImports`. Evidence: files
  changed + test results above.
- **Pin 1 (Type-1 D5)** — `Dispatch` returns the error chain carrying the
  underlying `*model.Error`; `model.IsTerminal` fires for 401/402. Evidence:
  `inprocess_test.go:TestInprocessTerminalErrorsPreserveModelError`.
- **Pin 2** — shared `driver.ErrKind*` constants; explicit
  `KindAuth → ErrKindAuth` switch arm. Evidence:
  `inprocess.go:errKindFromModel`.
- **Pin 6** — classified provider error on the verdict call keeps its real
  `ErrKind`. Evidence: `inprocess_test.go:TestInprocessVerdictProviderErrorKeepsKind`.
- **Pins 4+5** — `status.json.design_decisions` populated (D1/D5 Type-1
  Coach-decided); `CostSource="estimated"`. Evidence: `status.json`,
  `inprocess.go:economics`.
- **D7 guards** — config/protocol fail-closed guards proven. Evidence:
  `inprocess_test.go:TestInprocessConfigGuards`,
  `TestInprocessVerifierRequiresStructuredOutput`.
- **D1 identities** — `oai-inprocess` / `oai-responses-inprocess` off one
  struct, implementer+verifier declared, captain not. Evidence:
  `inprocess_test.go:TestInprocessIdentities`.

## Not delivered

- AC-03 test coverage for the `oai-responses-inprocess` identity —
  moot-by-construction (the Responses wire format emits tool-calling turns as
  pure `function_call` input items and cannot express the tool-only
  content-drop); fix location would be out-of-scope `internal/model`.
  Tracking + acknowledgement: captain-proceed.md pin 3 (Brad, Coach,
  2026-07-10).

## Divergence from plan

1. **Placement**: driver lands in subpackage `internal/driver/inprocess/`
   instead of the touchpoints' literal `internal/driver/inprocess.go` —
   ADR-0012 + S01's `TestNoWireImports` forbid wire imports anywhere in the
   contract directory, so the literal path could never pass AC-05's own test
   command. Full rationale: `journal.md`; recorded as a Type-2 decision in
   `status.json`. Planner note: S08's shared-touchpoint reference should read
   `internal/driver/inprocess/inprocess.go`.
2. **llm-check skipped** on the spec-v1 format lag (see First-pass section);
   manual AC-to-test cross-check substituted, per S02 precedent.
