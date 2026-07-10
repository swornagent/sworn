# Journal — S04-inprocess-oai-driver

## 2026-07-10 — Implementation session (design_review → in_progress → implemented)

Coach acknowledgement (`captain-proceed.md`, PROCEED, 6 pin dispositions) was
already committed; implementation proceeded against it. `start_commit`
6ce9d30526cd8eb1911fa8916ad8df8a8a117fed.

### Divergence: driver lives in subpackage `internal/driver/inprocess/`, not `internal/driver/inprocess.go`

Discovered at implementation start: the spec's touchpoints name
`internal/driver/inprocess.go` (files in the `driver` package directory), but

- ADR-0012 records, as part of the ratified Type-1 contract decision:
  "`internal/driver` itself imports neither `internal/model` nor
  `internal/agent` (enforced by `TestNoWireImports`)... so the contract
  package stays provider-neutral"; and
- `internal/driver/imports_test.go` (S01, T1-contract, merged) enforces that
  over **every** `.go` file in the directory.

A same-directory `inprocess.go` importing `internal/agent` + `internal/model`
(which this driver, by its whole purpose, must) fails `TestNoWireImports` —
i.e. the spec's own AC-05 test command `go test ./internal/driver/...` can
never pass with the literal touchpoint paths. The design review (pin 2
reasoned from same-package placement) did not catch this conflict.

Resolution chosen: the driver lives in **`internal/driver/inprocess/`**
(package `inprocess`) — files `inprocess.go`, `inprocess_verify.go`,
`inprocess_test.go`. Rationale, in constraint order:

1. It preserves the ADR-0012 Type-1 invariant and S01's landed enforcement
   test untouched. The alternative (exempting `inprocess*.go` inside
   `imports_test.go`) would weaken a Coach-ratified architectural guarantee,
   make every consumer of the contract package transitively import the wire
   packages, and require editing a file outside this slice's touchpoints —
   a strictly graver violation, and a Type-1 change only the Coach may record.
2. AC-05's own wording survives intact: the diff touches only
   `internal/driver` files (one level deeper), and the spec's literal test
   command `go test ./internal/driver/...` covers the subpackage.
3. Every pin disposition survives: the shared ErrKind vocabulary is consumed
   via the exported `driver.ErrKind*` constants (pin 2 — explicit
   `KindAuth → ErrKindAuth` mapping in `errKindFromModel`); `Dispatch`
   returns the error chain carrying the underlying `*model.Error` (pin 1);
   AC-03 narrowed to the chat identity (pin 3); `CostSource: "estimated"`
   (pin 5); pin-6 narrowing implemented as `classifyVerdictErr`.

Follow-on note for the planner (Rule 2, acknowledged here and in proof.json
divergence): S08-honest-cost-telemetry's touchpoint matrix lists
`internal/driver/inprocess.go` as a shared file; the real path is now
`internal/driver/inprocess/inprocess.go`. Tracking: owning slice
S08-honest-cost-telemetry (its planner/implementer must read this journal
entry via the touchpoint matrix); no code change required, path-only.

### Decisions and trade-offs (implementation-level)

- **Pin 1 mechanics.** `Dispatch` returns the loop's error chain (e.g.
  `agent: turn N: <*model.Error>`) rather than unwrapping and returning the
  bare `*model.Error`. `model.IsTerminal`/`model.AsError` walk the Unwrap
  chain, so the engine's terminal-halt fires identically (proven by
  `TestInprocessTerminalErrorsPreserveModelError` asserting
  `model.IsTerminal(err)` for 401 and 402), while the turn context stays in
  the message.
- **Verifier nudge turn.** The single `ChatStructured` verdict call appends
  one final `user` turn ("emit your verdict now") after the replayed
  transcript — some providers reject or mishandle a conversation ending on
  an `assistant` turn. The schema constraint still travels as
  `response_format`/forced-tool, never prose.
- **ResultText on the verifier path** carries the investigation loop's final
  text; the verdict JSON is in `StructuredJSON`. Both populated (contract:
  ResultText "always populated when available").
- **Verdict-call tokens** are added to the meter's totals via the same
  `observe` used for loop turns, so verifier economics include the final
  structured call.
- **StructuredOutput assert runs before the investigation loop** (fail-closed
  by construction, D7): a client that can chat but cannot emit a verdict is
  rejected before any tokens are spent
  (`TestInprocessVerifierRequiresStructuredOutput` proves Chat is never
  called).
- **Test seam.** `InProcess.newClient` (unexported) defaults to
  `model.NewClient`; tests inject a factory pointing the real `model.OAI`
  client at an `httptest` server, so every test exercises the full
  Dispatch → agent.Run → wire → tool-executor path (real `bash` tool
  execution in a real `git init` worktree) without any paid dispatch and
  without editing `internal/model` (AC-05).
- **Timeout default** 300s when `DispatchInput.Timeout` is zero, mirroring
  the subprocess drivers.

### AC-03 scope note (pin 3)

Per the Coach acknowledgement, AC-03's test obligation is narrowed to the
`oai-inprocess` (chat/completions) identity; `oai-responses-inprocess` is
exempt as moot-by-construction (`convertMessages` emits tool-calling
assistant turns as pure `function_call` input items — the Responses wire
format structurally cannot express the tool-only content-drop). Recorded
here, not silently skipped.

### Gates

- `go build ./...`, `go vet ./internal/driver/...`, `gofmt -l` clean; the
  newline-corruption grep over the new files found nothing.
- `go test -timeout 120s ./internal/driver/...` green (both packages,
  including the untouched `TestNoWireImports`).
- Full `go test -timeout 300s ./...` green — every package ok, zero
  failures.
