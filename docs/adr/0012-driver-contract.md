# ADR 0012: Driver contract — role-dispatch with a declared RoleSet

## Status

Accepted (2026-07-02) — decided by the Coach (Brad) during release
`2026-06-28-driver-contract` planning (recorded in that release's
`S01-driver-contract/status.json` `design_decisions[0]` and `intake.md`
"Driver contract shape: role-dispatch"). **Type-1 decision (Baton Rule 9):**
structural and hard to reverse — this contract is the single seam every
loop-role dispatch crosses; changing its shape after S02-S04 (subprocess and
in-process drivers) implement against it means unwinding three
implementations, not one.

## Context

`sworn`'s loop dispatches three roles (implementer, verifier, captain) to a
model or agent CLI. Today that dispatch goes through two separate,
loosely-coupled seams rather than one contract:

- `internal/run.RunSliceOptions.NewAgent` / `NewVerifier` — factory function
  fields with nil defaults, each producing a different interface
  (`agent.Agent` vs `model.Verifier`).
- `internal/verify.RunAgentic`'s `verifierAgent.(model.StructuredOutput)`
  type-assert — capability (does this driver support structured output?) is
  discovered at the call site, at runtime, rather than declared up front.

Neither seam has a place for a driver to say "I can serve these roles and no
others" before a dispatch is attempted. A driver that advertises a
capability it doesn't actually have — or is asked to serve a role it was
never built for — is only caught when the type-assert fails or the call
errors, not before the attempt is made.

## Decision

**Role-dispatch with a declared `RoleSet`.** A driver is:

```go
type Driver interface {
    Name() string
    Roles() RoleSet
    Dispatch(ctx context.Context, in DispatchInput) (Result, error)
}
```

Capability IS the declared role set: resolution calls `Roles().Has(role)`
before ever calling `Dispatch`, so an incapable driver is rejected by name at
resolution time — never discovered mid-run by a type-assert or a toolless
dispatch.

Four clauses decided in the same planning session travel with this shape:

1. **Role-universality.** Any driver may serve any loop role it declares —
   there is no role-specific driver type. A single `claude-subprocess`
   driver, for example, can serve implementer, verifier, and captain if it
   declares all three; the contract does not privilege one role's shape over
   another's.
2. **Engine-owned verdict validation.** `DispatchInput.VerdictSchema` carries
   the verdict JSON schema for a `Role=verifier` dispatch; the driver returns
   the model's verdict as `Result.StructuredJSON` and never validates or
   self-certifies it. The ENGINE validates `Result.StructuredJSON` against
   `verifier-verdict-v1`, fail-closed, after `Dispatch` returns. A driver
   that could mark its own verdict valid would defeat the fail-closed
   contract the verdict schema exists to enforce.
3. **Explicit-table registration with an enumeration API.** Drivers register
   into an explicit table (a later slice, S05); there is no reflection-based
   auto-discovery. The table exposes an enumeration API so the engine (and
   `sworn doctor`) can list every registered driver and the roles it
   declares.
4. **Explicit prefix-based resolution, no smart fallback.** A model ID
   resolves to a driver via an explicit prefix table (S05), not a
   best-effort or capability-sniffing fallback. A prefix that matches no
   registered driver fails closed rather than silently routing to a default
   — the silent-reroute-fallback lesson from swornagent/sworn#69.

The one-shot `model.Verifier` interface (used by non-loop utility gates —
`sworn verify`, `reqverify`, `llm-check`, `bench`) is explicitly **not**
touched by this decision; it survives untouched as the utility-judgement
path, separate from the loop's role dispatch.

Wire types (`model.ChatMessage`, `model.StructuredOutput`, `agent.Agent`,
etc.) become internal implementation details of in-process drivers (S04);
`internal/driver` itself imports neither `internal/model` nor
`internal/agent` (enforced by `TestNoWireImports`,
`internal/driver/imports_test.go`), so the contract package stays
provider-neutral regardless of which wire library a given driver wraps.

## Options considered

- **Role-dispatch with declared RoleSet (chosen).** One `Dispatch` method,
  capability declared and checked at resolution. Matches role-universality
  exactly and closes the class of bug where a driver's actual capability is
  discovered mid-run rather than declared up front.
- **Minimal Dispatch core + optional interfaces discovered by type-assert**
  (today's `RunAgentic` pattern generalised). Rejected: this is the shape
  that already produces the mid-run-discovery failure class described above
  — generalising it does not fix the underlying problem, it spreads it.
- **Maximal: fold non-loop utility judgements (gates/bench) into Dispatch
  too.** Rejected: `model.Verifier`'s one-shot utility path serves a
  genuinely different call shape (single fresh-context judgement, no
  role/worktree/timeout structure) and folding it in would either bloat
  `DispatchInput` with loop-only fields or force the utility path through
  loop machinery it doesn't need.

## Consequences

- `internal/driver` is a new leaf package: `Driver`, `RoleSet`,
  `DispatchInput`, `Result`, `AssertWorktree`. No driver implements it yet;
  nothing calls `Dispatch` yet — this ADR lands the shape, not the rewire.
- S02 (claude subprocess) and S03 (codex subprocess) implement `Driver`
  against this contract; S04 (in-process OAI-compatible driver) does the
  same and is where the wire-type wrapping actually happens.
- S05 (driver registry) owns the explicit-table registration and
  enumeration API; S06/S07 rewire `internal/run` and `internal/verify` off
  `NewAgent`/`NewVerifier`/the `StructuredOutput` type-assert and onto
  `Driver.Dispatch`.
- S10's conformance suite asserts these behavioural clauses
  driver-agnostically, so any future driver (not just the three landing in
  this release) is checked against the same contract.
- A shape change discovered while implementing S02-S04 routes back through a
  replan, not a silent edit — this ADR is the record a replan would revise.

## References

- Release `2026-06-28-driver-contract`, slice `S01-driver-contract`
  (`spec.json`, `status.json design_decisions[0]`, `design.md`).
- `swornagent/sworn#69` — the silent-reroute-fallback lesson behind clause 4.
- `internal/verify/verify.go` (the `verifierAgent.(model.StructuredOutput)`
  type-assert this contract replaces) and `internal/run/slice.go`
  (`RunSliceOptions.NewAgent`/`NewVerifier`) — the seam this ADR supersedes,
  rewired in T4 (S05-S08).

## Amendment (2026-07-12) — role-agnostic StructuredSchema + ErrKindUnsupported

Release `2026-07-11-loop-operability`, slice `S02-model-response-structured`.
**Type-1, architecturally significant** — decided by the Coach (Brad),
`captain-proceed.md` pins 1 and 3; recorded in that slice's `status.json`
`design_decisions` (D1, D3). This amends the contract shape landed above; it
does not supersede the ADR.

Two prose-scraping loop gates — the design-TL;DR gate (`internal/design`, which
required literal `§1`–`§6` headers) and the reqverify Definition-of-Ready gate
(`internal/reqverify`, which scraped a `## RESULTS` prose section) — are
migrated onto the same schema-constrained structured-output transport the
verifier verdict already uses. Both are captain-family dispatches, so the
contract change is:

1. **`DispatchInput.VerdictSchema` → `DispatchInput.StructuredSchema`
   (role-agnostic).** The driver enforces an output schema; it does not care
   which role asked. The verifier passes verifier-verdict-v1; the captain-family
   gates pass their own sworn-local emit schemas. Chosen over a parallel
   `CaptainSchema` field: two fields for one concept invite drift, and the
   field's own doc already anticipated generalisation. The rename is
   compile-checked across every call site (`driver.go`, `claude.go`,
   `codex.go`, `verify.go`, `inprocess*.go`, `drivertest/conformance.go`, and
   the tests). `dispatchCaptain` gains a structured path (one ChatStructured
   call, no investigation loop) taken when `StructuredSchema` is set; the prose
   `Chat` path is unchanged when it is nil.

2. **New `ErrKindUnsupported = "unsupported"` in the binding cross-driver
   ErrKind taxonomy (`internal/driver/subprocess.go`).** A schema-constrained
   dispatch to a client that cannot emit structured output fails closed with
   THIS kind — deliberately distinct from `ErrKindProtocol` (a structured
   *emission* that failed). Capability-absent is not a failure to retry but a
   **declared Rule 2 deferral** the gate records; every other structured
   failure stays a hard, fail-closed error. It is NOT terminal —
   `TerminalErrKind` stays `{auth, credits}`. Per `[[project_driver_contract_recut]]`
   the ErrKind vocabulary binds for all future drivers, so subprocess-family
   drivers map capability-absent to this kind too rather than folding it into
   `ErrKindProtocol`.

Consequence: a `StructuredOutput`-capable model that does not reproduce the
exact prose shape the gates were tuned against (as Grok did not) now passes
both gates; a model that genuinely cannot emit structured output degrades to a
declared deferral naming the missing capability, never a silent pass and never
a hard prose-format failure.
