# Design TL;DR — S01-driver-contract

## Approach

Add a new leaf package `internal/driver` containing only the contract: types,
one Rule-11 assertion helper, and their tests. No driver implements it yet
(S02/S03/S04); nothing calls it yet (T4 rewires `internal/run` and
`internal/verify` onto it later). This slice's job is to fix the shape so
those downstream slices build against a stable target instead of guessing.

```go
package driver

type Role string

const (
    RoleImplementer Role = "implementer"
    RoleVerifier    Role = "verifier"
    RoleCaptain     Role = "captain"
)

type RoleSet map[Role]bool
func (s RoleSet) Has(r Role) bool
func (s RoleSet) String() string // names the declared roles, deterministic order

type DispatchInput struct {
    Role          Role
    ModelID       string
    SystemPrompt  string
    Payload       string
    WorktreeRoot  string
    VerdictSchema []byte
    Timeout       time.Duration
}

type Status string
const (
    StatusOK      Status = "ok"
    StatusBlocked Status = "blocked"
    StatusError   Status = "error"
)

type Result struct {
    Status         Status
    ErrKind        string // set when Status == StatusError
    ResultText     string
    StructuredJSON json.RawMessage
    CostUSD        float64
    CostSource     string
    InputTokens    int64
    OutputTokens   int64
    ModelID        string
    DurationMS     int64
}

type Driver interface {
    Name() string
    Roles() RoleSet
    Dispatch(ctx context.Context, in DispatchInput) (Result, error)
}

func AssertWorktree(path string) error
```

`AssertWorktree` walks: path exists → is a directory → `git rev-parse
--is-inside-work-tree` (or equivalent stat-based check) succeeds from that
path — each failure names the path and which check failed, per AC-04.

## Key design choices + rationale

1. **Role-dispatch shape, not core+optional-interfaces.** Already decided
   Type-1 by Brad (2026-07-02, recorded in `status.json` `design_decisions[0]`
   and `intake.md`). This design.md does not re-litigate it — it implements
   the recorded decision. Restated here for the reviewer's convenience: a
   driver declares its `RoleSet` up front, so `Roles().Has(Role)` at
   resolution time is what rejects an incapable driver — never a type-assert
   discovered mid-run. That closes the exact class of bug in sworn#35
   (Claude subprocess driver advertised structured output it didn't have).

2. **Wire types stay internal to in-process drivers.** `DispatchInput` /
   `Result` carry only primitives, `json.RawMessage`, and stdlib types
   (`time.Duration`, `context.Context`) — no `model.ChatMessage`, no
   `agent.Agent`. `internal/model`'s `ChatMessage`/`StructuredOutput`/
   `Verifier` types (`internal/model/client.go:52-61`) become an
   implementation detail of the in-process driver (S04) that wraps them; the
   contract package never imports `internal/model` or `internal/agent`.
   AC-05's `TestNoWireImports` enforces this at the AST level so a future
   edit can't reintroduce the coupling silently.

3. **Engine owns verdict validation; the driver never self-certifies.**
   `DispatchInput.VerdictSchema` is opaque bytes in, `Result.StructuredJSON`
   is opaque bytes out. The engine (not this package) runs the
   `verifier-verdict-v1` fail-closed check — mirroring the existing
   `internal/verdict` validator and the `so.ChatStructured(...)` call site in
   `internal/verify/verify.go:200`, which is exactly the type-assert seam T4
   replaces. Documented as a MUST in both the `Driver.Dispatch` godoc (AC-03)
   and ADR-0012 (AC-06) so a future driver author doesn't try to validate
   client-side.

4. **No registration/resolution logic in this slice.** `RoleSet.Has` is the
   only resolution-relevant primitive; explicit-table registration and
   prefix-based resolution (the other clauses from the same planning session)
   are S05's job, not this slice's. Keeping this package leaf-only is what
   makes the import-boundary test meaningful — there is nothing here to wire
   up yet.

## Files touched

Exactly the slice's declared touchpoints — no orchestrator or `internal/run`
edits in this slice:

- `internal/driver/driver.go` — `Role`, `RoleSet`, `DispatchInput`, `Status`,
  `Result`, `Driver` interface, godoc (AC-01, AC-02, AC-03).
- `internal/driver/result.go` — `Result` helper methods if any prove useful
  (e.g. `IsTerminal() bool` grouping `blocked`/`error`); kept minimal, added
  only if a test needs it — not speculative.
- `internal/driver/worktree.go` — `AssertWorktree` (AC-04, Rule 11).
- `internal/driver/driver_test.go` — `TestRoleSet` (AC-02), `TestAssertWorktree`
  (AC-04).
- `internal/driver/imports_test.go` — `TestNoWireImports` (AC-05).
- `docs/adr/0012-driver-contract.md` — Type-1 record: options considered,
  decision, decider, the four clauses (AC-06).

## Design-level risks / pins

- **R-01 (spec)**: shape might turn out claude-cli-shaped and not generalise
  to codex/in-process. Mitigation is downstream (S02-S04 implement against
  it before T4 consumes it) — nothing to pin in this slice beyond keeping
  `DispatchInput`/`Result` provider-neutral (no claude-cli-specific fields
  snuck in). Flagging for the reviewer: watch for any field that only makes
  sense for a subprocess driver.
- **R-02 (spec)**: scope creep into a driver implementation or the
  orchestrator. Touchpoints above are the enforcement; `imports_test.go`
  backstops it structurally.
- **AC-01 exact shape is load-bearing.** This is the keystone contract per
  `status.json.effort_complexity` ("low effort, high complexity — puzzle").
  A shape error here is expensive to unwind once S02-S04 are implemented
  against it. Reviewer should read AC-01's field list against the Go snippet
  above line by line before acknowledging.

## AC traceability

| AC | Delivered by |
|----|---|
| AC-01 | `driver.go` type definitions |
| AC-02 | `driver.go` (`RoleSet.Has`/`String`) + `driver_test.go` (`TestRoleSet`) |
| AC-03 | godoc on `Driver.Dispatch` + ADR-0012 |
| AC-04 | `worktree.go` (`AssertWorktree`) + `driver_test.go` (`TestAssertWorktree`) |
| AC-05 | `imports_test.go` (`TestNoWireImports`) |
| AC-06 | `docs/adr/0012-driver-contract.md` + `go build ./...` / `go test ./internal/driver/...` |
