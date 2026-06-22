---
title: 'S58-slice-router — deterministic slice router (captain-route.sh port)'
description: 'sworn route <slice> <release> computes the next command for a slice purely from its committed status.json, faithfully porting captain-route.sh: the state machine + the design-review/Gate-re-entry/merge decisions. No LLM.'
---

# Slice: `S58-slice-router`

> Proposed by the 2026-06-23 port-fidelity audit (`internal-docs/captures/2026-06-23-port-fidelity-audit/08-router-and-dispatch.md`). The orchestration brain: `captain-route.sh` is the router + the (S57) oracle reader + the design-review/Gate-re-entry/merge state machine. This slice ports the **decision tree**; it consumes S57 for state reads. Depends on **S57-oracle-reader**.

## User outcome

A developer (or the autonomous loop) runs `sworn route <slice-id> <release-name>` and receives a deterministic JSON decision — `next.type`, `next.command`, `next.reason`, plus the slice's resolved `state` and `verification` — computed purely from the slice's committed `status.json` (via S57). The output matches `~/.claude/bin/captain-route.sh` on the JSON `.next` contract for every state. No LLM is invoked.

## Entry point

`sworn route <slice-id> <release-name> [--pretty]` — subcommand on `cmd/sworn/route.go`, self-registered via `init()` calling the S51 command registry (never edits `main.go`). Mirrors `captain-route.sh`'s CLI so bash and Go forms are interchangeable.

## In scope

- `internal/router/router.go` (new): pure `Route(ctx, oracle board.OracleReader, sliceID, release string) (Decision, error)` — no I/O except through the injected reader; deterministic, table-testable.
- The full `(state, verification.result)` → `next` decision tree, ported faithfully from `captain-route.sh:248-549`:
  - `verification.result == blocked` (any state) → `replan-release` (BLOCKED precedes state).
  - `design_review` → sub-states by **commit-time-newest** artefact: `approved-ack.md` present → `implement`; newest of {`design.md`,`decline.md`,`review.md`} → `review` / `implement` / `coach_decision` (the S06 150× overnight-spin guard — route on commit time, not mere presence).
  - `failed_verification` → classify `verification.violations` by Gate: any Gate 1/2/6 → `redesign` (signals caller to remove `approved-ack.md` so the design gate re-fires); else Gate 3/4/5 → `implement`.
  - `implemented` → `verify` (always — survives crashed/killed verifiers; `pending`/stale results re-verify).
  - `in_progress` → `implement` (synchronous loop ⇒ in_progress = died mid-flight; resume).
  - `planned` → `implement` (Design TL;DR gate halts before code).
  - `shipped` → `none`.
  - `verified` → walk this track's slice order (ownership/ghost-filtered via S57) for the next non-terminal slice → route to it (`implement`, or `review` if it has a `design.md`); if the track is fully terminal, decide `merge-track` (peers in flight, or this track unmerged into release-wt by git-ancestry) vs `merge-release` (all slices terminal AND all tracks merged).
  - unrecognised state → `none` with a manual-inspection reason.
- `Decision` struct + JSON marshalling producing the exact `captain-route.sh` shape (`:555-598`); `next.type` enum: `implement | review | verify | merge-track | merge-release | replan-release | redesign | coach_decision | none`.
- Tolerant `verification.reason`: `.verification.reason` else `(.verification.violations | join("; "))` (the 2026-06-10 S06 find).
- `cmd/sworn/route.go` (new): wires flags, calls S57 reader + the router, prints compact JSON (default) or coloured `--pretty`. Timestamps (`generated_at`) are stamped here, never inside `Route`.

## Out of scope

- **S57-oracle-reader** — the git-ref reader. This slice CONSUMES it via `board.OracleReader`; it is a prerequisite, not built here.
- **S59-scheduler-relayer** — the poll-and-route loop that *dispatches* `next.type`. `route` only decides; it never executes a command.
- `dispatch_and_interpret` / the LLM verdict interpreter, the runtime-drivers dispatch boundary, ntfy paging — separate concerns.

## Planned touchpoints

- `internal/router/router.go` (new)
- `internal/router/router_test.go` (new)
- `internal/router/parity_test.go` (new)
- `cmd/sworn/route.go` (new)
- `cmd/sworn/route_test.go` (new)

## Acceptance checks

- [ ] `planned` slice routes `implement` (`/implement-slice <slice> <release>`).
- [ ] `implemented` (no verdict) routes `verify`; `verification.result=pending` and stale `fail`/`pass` also route `verify`.
- [ ] `verification.result=blocked` routes `replan-release` regardless of `state`.
- [ ] `failed_verification` with a Gate 1/2/6 violation routes `redesign`; with only Gate 3/4/5 routes `implement`.
- [ ] `design_review` routes by commit-time-newest artefact: `approved-ack.md` → `implement`; `review.md` newest → `coach_decision`; `decline.md` newest → `implement`; `design.md` newest → `review`. (S06 overnight-spin regression guard.)
- [ ] `verified` with a later `planned` sibling routes to it (`review` if it has `design.md`, else `implement`); with all siblings terminal routes `merge-track` (peers ongoing) or `merge-release` (all terminal + all tracks merged via git-ancestry).
- [ ] Ghost-slice filter: a `verified` slice whose track's later entry is owned by another track skips that ghost.
- [ ] `shipped` routes `none`; unrecognised state routes `none` with a manual-inspection reason.
- [ ] **Parity test**: for a fixture release covering every state, Go `sworn route` JSON `.next` equals `captain-route.sh`'s `.next` (golden; `generated_at` excluded).
- [ ] `go test -race ./internal/router/...` passes; `Route` does no I/O except via the injected reader (fake reader in tests).

## Required tests

- **Unit**: `internal/router/router_test.go` — table-driven per state branch; `TestBlockedPrecedesState`, `TestDesignReviewCommitTimeNewest`, `TestFailedVerificationGateClassification`, `TestVerifiedWalksTrackThenMerges`, `TestGhostSliceFiltered`.
- **Integration / Reachability artefact (Rule 1)**: `cmd/sworn/route_test.go` runs the real `sworn route` subcommand against a committed fixture and asserts the JSON decision — reachability is the CLI command, not a leaf call.
- **Parity (golden)**: `internal/router/parity_test.go` — run `captain-route.sh` (skip with a clear message if not on PATH) and the Go router over the same fixture refs; assert `.next` equality. `captain-route.sh` is the literal oracle of correctness for the port.

## Risks

- **Losing the scar tissue.** The value is the encoded edge cases, not the happy path. The parity test + the named regression checks (S06 commit-time, S12 in_progress-resume, ghost filter, transient-read via S57) are the defence — none may be dropped. A "cleaner" rewrite that routes `design_review` on presence not commit time reintroduces the 150× spin.
- **Reader semantics live in S57.** If S57 reads the working tree, the router inherits the stale-read trap. The `OracleReader` contract specifies committed-ref reads; router correctness depends on it.
- **Non-determinism.** `Route` must be pure; `generated_at` is stamped at the CLI edge so unit tests stay deterministic.

## Deferrals allowed?

No. The router is the keystone of the orchestration port; its edge-case fidelity is the whole point.
