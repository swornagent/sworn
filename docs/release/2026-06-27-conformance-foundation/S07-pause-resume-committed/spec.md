---
title: 'S07 — Resume seeds the frontier from committed state (terminal-set unified)'
description: 'Make findFirstNonTerminal seed the resume frontier from committed git-visible state using the SAME terminal-set as the router (verified/shipped/deferred), so implemented-but-unverified slices are re-verified rather than abandoned; centralise the terminal-set in one helper; and fix the commented-out finishTrack return that strands a fully-terminal track on resume.'
---

# Slice: `S07-pause-resume-committed`

## User outcome

When a `sworn run --parallel` session is interrupted (crash, pause) and then resumed, the loop re-attaches to each track at the correct frontier slice: it re-verifies an `implemented`-but-unverified slice instead of skipping past it, and a track whose slices are all terminal merges cleanly instead of failing. The resume frontier is computed from committed (git-visible) state, consistent with the router, so a dirty or partially-written working-tree never changes which slice resumes.

## Background — why this slice was re-scoped (Coach decision 2026-06-28)

The original spec diagnosed the defect as "`findFirstNonTerminal` reads the working-tree copy and re-runs the wrong slice." The Captain design review (NEEDS_COACH) and a code re-audit found that premise is false, and that following it would introduce a regression:

- **`findFirstNonTerminal` reads no state today.** It returns `slices[0]` unconditionally (`internal/scheduler/worker.go:536`). It is only the *seed* for the first `Route()` call; the authoritative state machine is the router (`internal/router/router.go`), which walks **committed** git refs via `oracle.ReadSliceStatus` (track → release-wt → HEAD priority chain). The "dirty working-tree re-runs the wrong slice" bug does not exist in the frontier-selection path — the only working-tree `state.Read` calls are inside the per-slice implement/verify loop, not frontier selection.
- **The original AC2 ("skip past `implemented`") was a regression.** The frontier walk is forward-only (`routeVerified`, router.go:271, never returns). If the seed skips an `implemented` slice it lands on a *later* slice and the router can never return to verify the skipped one — so an `implemented`-but-unverified slice (crashed before verification) would be **abandoned**, the opposite of this slice's goal. The router already treats `implemented` as **non-terminal** (`routeImplemented`, router.go:251, drives `implemented → /verify-slice`).
- **A real, confirmed bug exists nearby.** At `internal/scheduler/worker.go:232` the all-terminal early return is fused onto its comment line — `// All slices already in a terminal state.		return finishTrack(...)` — so the `return` is commented out and the `if currentSlice == "" {…}` block is empty. It is dead today (findFirstNonTerminal never returns `""`), but making the seed read committed state (this slice) makes `""` reachable, at which point a fully-terminal track on resume falls through, is polled with an empty sliceID, and returns `TrackFail` instead of merging.

**Coach decision (Brad, 2026-06-28): replan S07 properly.** Seed from committed state; treat `implemented` as non-terminal; unify the terminal-set in one helper shared by router and scheduler; fix the `finishTrack` fused-line bug. See `## Coach decision` below.

## Entry point

`sworn run --parallel [--resume]` → `internal/scheduler/worker.go` `runTrackRouter` (worker.go:230) which seeds `currentSlice` from `findFirstNonTerminal` (worker.go:536). Reachable via `cmd/sworn/run.go`.

## In scope

- `internal/router/router.go` (or a small new `internal/router/terminal.go`): export a single terminal-set predicate, e.g. `func IsTerminal(state string) bool` returning true for `{verified, shipped, deferred}`. Replace the two inline definitions at router.go:307 and router.go:393 with calls to it.
- `internal/scheduler/worker.go`:
  - `findFirstNonTerminal`: change signature to accept the oracle reader + release/track context; return the first slice whose **committed** state (via `oracle.ReadSliceStatus`) is non-terminal per the shared `router.IsTerminal`. `implemented`, `in_progress`, `planned`, `failed_verification` are all non-terminal (seeded). Returns `""` only when every slice is terminal.
  - Fix the **worker.go:232** fused line so the all-terminal branch actually executes `return finishTrack(...)` (restore the newline).
- `cmd/sworn/run.go`: confirm/define the `--resume` flag and its observable contract (see AC6). `--resume` without `--parallel` exits 64 (usage error).

## Out of scope

- Changing what the oracle reads from (index.md vs board.json — that is S14).
- The PAGE event path (S03), triage, model dispatch.
- The per-slice implement/verify loop's working-tree `state.Read` calls (`internal/run/slice.go`) — those are correct as-is for their purpose.

## Planned touchpoints

- `internal/router/router.go` (export IsTerminal; replace 2 inline terminal-sets)
- `internal/scheduler/worker.go` (committed-read seed; fix finishTrack fused line)
- `cmd/sworn/run.go` (confirm `--resume` flag + usage gate)

## Acceptance checks (EARS)

- [ ] **AC1 — committed seed.** WHEN `runTrackRouter` seeds the resume frontier, THE SYSTEM SHALL select the first slice whose **committed** (git-visible via `oracle.ReadSliceStatus`) state is non-terminal, NOT `slices[0]` unconditionally and NOT the working-tree copy.
- [ ] **AC2 — implemented is non-terminal (corrected).** WHEN a slice's committed state is `implemented`, THE SYSTEM SHALL treat it as non-terminal and seed the frontier at it, so the router routes it to `/verify-slice` (it SHALL NOT be skipped/abandoned).
- [ ] **AC3 — track-ref-unreadable fallback.** WHEN a slice's status cannot be read from the track branch because the track ref does not yet exist, THE SYSTEM SHALL fall back to the release-wt slice state (NOT error out, NOT read the working-tree). The implementer SHALL confirm `oracle.ReadSliceStatus` returns empty (→ fallback) rather than propagating an error for a nonexistent ref; if it errors, handle it so the fallback holds.
- [ ] **AC4 — all-terminal track merges on resume (real bug fix).** WHEN `findFirstNonTerminal` returns `""` (every slice terminal) on resume, THE SYSTEM SHALL merge the track via `finishTrack` (the worker.go:232 fused-line return SHALL execute), NOT fall through to a router poll with an empty sliceID.
- [ ] **AC5 — single terminal-set.** THE SYSTEM SHALL define the terminal-set `{verified, shipped, deferred}` in exactly one exported helper consumed by both the router (router.go:307 and :393) and the scheduler seed; there SHALL be no divergent terminal definition in the scheduler.
- [ ] **AC6 — `--resume` observable contract.** THE `--resume` flag SHALL have a stated observable effect: it re-attaches to an existing in-flight release and seeds each track from committed state (per AC1). `--resume` without `--parallel` SHALL exit 64. The spec/help text SHALL state whether the committed seed runs only under `--resume` or on every `--parallel` run (document the actual behaviour, no silent no-op flag).

## Required tests

- **Unit (AC1/AC2)**: `internal/scheduler/worker_test.go` — `TestFindFirstNonTerminalCommitted`: a mock oracle returns committed states `[verified, implemented, planned]`; assert the seed is the `implemented` slice (index 1), proving committed-read + implemented-non-terminal.
- **Unit (AC4)**: `TestFindFirstNonTerminalAllTerminalMergesTrack` — oracle returns all `verified`; assert `findFirstNonTerminal` returns `""` AND `runTrackRouter` reaches `finishTrack` (e.g. via a seam/fake `releaseTrack` callback or a `finishTrack` invocation assertion), proving the fused-line fix.
- **Unit (AC5)**: `internal/router/*_test.go` — `TestIsTerminal` table covering verified/shipped/deferred → true and planned/in_progress/implemented/failed_verification → false; and a guard that the scheduler imports the router helper (no second definition).
- **Reachability artefact**: `go test ./internal/scheduler/... ./internal/router/... -run 'TestFindFirstNonTerminal|TestIsTerminal' -v` exits 0.

## Coach decision

- **DD-1 (Type-1):** terminal-set semantics for the resume seed = `{verified, shipped, deferred}` (the router's definition); `implemented` is **non-terminal**. Type-1 because it is behaviour-shaping across the resume path and gates whether crashed-mid-verify work is recovered. Decided by Brad (Coach), 2026-06-28, replacing the original AC2.
- **DD-2 (Type-1):** unify the terminal-set in one exported `router.IsTerminal` helper consumed by both router and scheduler — no divergent set. Decided by Brad (Coach), 2026-06-28 (Captain pin 3).
- **DD-3 (Type-2):** `findFirstNonTerminal` signature gains the oracle reader + release/track context (already available in `runTrackRouter`); reversible, local.

## Risks

- The committed oracle read can fail in a detached HEAD / bare clone; AC3's release-wt fallback covers the common crash-recovery case. The implementer must confirm the nonexistent-ref path returns empty rather than erroring (Captain pin 5).

## Deferrals allowed?

No.
