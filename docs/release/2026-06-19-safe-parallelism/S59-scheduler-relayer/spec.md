---
title: 'S59-scheduler-relayer — router-driven worker loop (poll-and-route)'
description: 'sworn run --parallel workers stop iterating a static slice list and instead poll the router (S58) each step, dispatching its next.type, until the track is terminal or paused. Makes the loop resumable and dynamic while keeping RunParallel''s dependency resolution, worktree isolation, and supervisor ownership.'
---

# Slice: `S59-scheduler-relayer`

> Proposed by the 2026-06-23 port-fidelity audit (router/dispatch deep-read). Closes the orchestration-core port: replaces `RunParallel`'s **static-DAG execution heart** with a **router-driven poll loop**. Depends on **S58-slice-router** (and transitively S57). The wrap-vs-replace decision is this slice's Captain design-review pin (see Design decisions).

## User outcome

A developer runs `sworn run --parallel --release <name>`, and each track's worker drives its slices by **polling the router for the track's current committed state and dispatching the router's `next.type`**, looping until the track reaches a terminal state or a human-gated pause (`coach_decision` / `replan-release` / `error`). Killing and re-running `sworn run --parallel` **resumes from committed state** — already-`verified` slices are skipped, in-flight slices pick up where they left off — instead of re-planning from scratch.

## Entry point

`sworn run --parallel --release <release-name>` — same entry as S02b (`cmd/sworn/run.go`); behaviour of the worker loop inside changes. No new flag.

## Background

S02b's `RunParallel` is a **plan-then-execute** static-DAG executor: it topologically plans phases once, fans out, and each worker (`internal/scheduler/worker.go`) iterates a **fixed slice list** to terminal behind phase barriers, returning bare `error`. The reference coach loop is **observe-then-act**: each step derives the next action purely from the slice's committed `status.json` (via `captain-route.sh` = S58 here), so it is intrinsically **resumable** (every decision is a pure function of committed state) and **dynamic** (picks up replan-added slices, cleared deps, and failures on the next poll) — it keeps working whatever is workable until nothing non-terminal remains, pausing only for a human. This slice ports that execution model into the worker while **keeping** the parts of `RunParallel` worth keeping: `scheduler.BuildPlan` dependency resolution, worktree materialisation/isolation, and `supervisor` single-writer ownership (S01).

## In scope

- `internal/scheduler/worker.go` (re-layer): the per-track worker loop becomes — read the track's current frontier slice → call `router.Route(...)` (S58) → dispatch the returned `next.type` (`implement` → `run.RunSlice`; `verify` → the verify step; `redesign` → strip `approved-ack.md` then implement; `merge-track`/`merge-release`/`replan-release`/`coach_decision` → surface/pause per the loop's pause set) → repeat until the router returns a terminal/paused decision for the track. Keeps `supervisor.Acquire/Release` and worktree materialisation unchanged.
- `internal/run/parallel.go` (re-layer): keep `scheduler.BuildPlan` dependency ordering + the sequential release-wt pre-flight; replace the "run each slice once to terminal" inner contract with the router-driven worker; **resumability** — on (re-)entry a track whose frontier slice the router reports `verified`/`shipped` is skipped, not re-run.
- Preserve S02b's existing observable contract: per-track `[Tn]` stderr prefixes; exit code 0 only when all tracks reach terminal PASS; FAIL/dependency cancellation semantics (a track that the router routes to `replan-release`/`error` does not silently pass).

## Out of scope

- The router decision tree itself (S58) and the oracle reader (S57).
- `dispatch_and_interpret` / the LLM interpreter and the runtime-drivers dispatch-boundary contract (separate, post-T17 — flagged by the audit's `06`/runtime-drivers findings).
- ntfy/webhook paging wiring (S07, T3) — this slice surfaces pause states; it does not own the paging transport.
- Release-level circuit breaker / global cost ceiling (audit P1; separate slice — surfaced as a Rule-2 follow-on below).

## Design decisions (for the Captain review to ratify)

- **Wrap vs replace.** Proposed: **wrap** — keep `scheduler.BuildPlan` (dependency resolution), worktree isolation, and `supervisor` ownership; replace only the worker's *execution heart* (static slice iteration → router poll-and-route). Rationale: the reference's poll-and-route model carried 32 getfired releases and gives resumability for free; the DAG scaffolding is still worth keeping. Confirm vs a fuller replace of `RunParallel`.
- **Pause set.** Proposed human-gated `next.type` values that stop a track and surface (not fail): `coach_decision`, `replan-release`. `error`/exhausted → fail-closed surface. Confirm.

## Planned touchpoints

- `internal/scheduler/worker.go` (re-layer)
- `internal/scheduler/worker_test.go` (extend)
- `internal/run/parallel.go` (re-layer)
- `internal/run/parallel_test.go` (extend)

## Acceptance checks

- [ ] A worker drives a 2-slice track to completion by polling the router (not a static list): `planned`→implement→`implemented`→verify→`verified`, then advances to the next slice — asserted via a fake router + fake RunSlice recording the dispatch sequence.
- [ ] **Resumability**: re-invoking `sworn run --parallel` on a fixture where slice 1 is already `verified` skips slice 1 (no second implement/verify dispatch) and resumes at slice 2.
- [ ] A `redesign` router decision causes the worker to remove `approved-ack.md` before re-dispatching `implement`.
- [ ] A `coach_decision` / `replan-release` router decision pauses that track and surfaces it (no auto-pass, no infinite loop) while other tracks continue.
- [ ] `supervisor.Acquire`/`Release` still bracket every worker (normal + error paths); `go test -race ./internal/scheduler/... ./internal/run/...` passes with zero races.
- [ ] Exit code 0 only when every track reaches terminal PASS; a paused/failed track yields non-zero.

## Required tests

- **Unit**: `internal/scheduler/worker_test.go` — `TestWorkerPollsRouterDrivesSlice`, `TestWorkerResumesSkipsVerified`, `TestRedesignStripsAck`, `TestPauseStateSurfacesNoLoop` (fake router returning scripted decisions; fake RunSlice/verify recording dispatches).
- **Reachability artefact (Rule 1)**: smoke step — `sworn run --parallel --release <fixture>` on a 2-track fixture, kill mid-run, re-run, observe the second run skip the already-`verified` slice (resumability). Document the two-run transcript in `proof.md`.

## Risks

- **Touchpoint collision with T12 / T13.** `internal/run` + `internal/scheduler` are also touched by T12 (S42-S44 run-loop changes) and T13/S47 (`slice.go`). T17 `depends_on T12`, and re-scoped S47 `depends_on T17` (S58), serialise these — this slice must land after T12 merges. A `/merge-track` conflict here is a planner-ordering error (track-mode invariant 4), not a code conflict to resolve ad hoc.
- **Resumability correctness depends on S57/S58 reading committed state.** If the worker polls working-tree state instead of the committed oracle, a dirty mid-run tree mis-routes. The router (S58) reads via S57's committed-ref reader — keep that path; do not shortcut to `state.Read`.
- **No release-level circuit breaker yet** (Rule 2 follow-on): a global cost/failure ceiling across the fan-out is out of scope here and tracked as a separate slice; without it, per-slice caps are the only bound. *Why:* keep this slice to the execution-model change; *tracking:* separate audit-P1 slice; *ack:* Coach 2026-06-23.

## Deferrals allowed?

Yes, with Rule 2 — the release-level circuit breaker and the runtime-drivers dispatch-boundary conformance carry forward with why/tracking/ack (above + audit `06`/runtime-drivers). The router-driven worker loop + resumability is the landing scope.
