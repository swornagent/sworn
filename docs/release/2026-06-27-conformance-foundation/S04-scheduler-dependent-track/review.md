# Captain review — S04-scheduler-dependent-track
Date: 2026-06-28
Design commit: 13cd4deaa879fef4f912a0715a03763bd82788fe

## Drift gate note

Track `T1-orchestration` is 1 commit behind `release-wt/2026-06-27-conformance-foundation`. The
lagging commit (`a061677 replan: T3 & T7 depend_on T2-model-layer`) only modifies
`docs/release/2026-06-27-conformance-foundation/index.md` to record cross-track dependencies for
T3 and T7 — it does not touch any T1 or S04 artefact. S04's `spec.md` and `design.md` are not
stale. Review proceeds against the current track state.

## Pins

1. [mechanical] §1/AC1,AC4 — `waitForDependencies` will deadlock in production; the oracle never transitions to "merged" from a bare auto-merge
   What I observed: The design proposes a `waitForDependencies` poll loop inside `RunTrack` that
   calls `board.OracleReader.ReadBoard` and waits for `ts.State == "merged"` for each `DependsOn`
   track. `board.Oracle.ReadBoard` reads the track's `state:` field from `index.md` on the
   `release-wt/<release>` git ref (confirmed in `internal/board/oracle.go:408-413`). The design's
   `MergeTrackFn` is described as running `git merge --no-ff <track-branch> --no-edit` in
   `ReleaseWorktreePath`. That git-merge commit updates the release-wt branch with the track's
   code, but does NOT update `index.md` to set `state: merged`. After auto-merge, `ReadBoard` still
   returns the dependency track's state as "in_progress". `waitForDependencies` polls forever →
   deadlock under any release with a `depends_on` edge. This is the same root cause for both AC1
   and AC4: the anti-deadlock mechanism (`ctx.Done()` termination) cannot fire because the poll
   loop is checking a value that never changes, not a timeout. Additionally, the `BuildPlan` /
   `RunParallel` phase barrier already guarantees AC1: dependent tracks are placed in a later
   phase and their goroutines are not started until `wg.Wait()` returns for the prior phase (all
   dependency-phase goroutines have returned). `waitForDependencies` is redundant with the phase
   barrier AND introduces a deadlock.
   What to ask the implementer: Choose ONE resolution and apply it inline during implementation:
   (a) DROP `waitForDependencies` entirely and document in a code comment that the phase barrier
   in `RunParallel` (`wg.Wait()` per phase) already enforces AC1 — no polling needed. Update
   the AC traceability table accordingly: AC1 → phase barrier; AC4 → phase barrier + `ctx.Done()`
   in the existing worktree materialisation path. (b) KEEP `waitForDependencies` but have
   `MergeTrackFn` ALSO update `index.md` on `release-wt` to set the track's `state: merged`
   before returning — the oracle then reflects the change immediately. If (b), the `MergeTrackFn`
   signature `func(releasePath, branch string) error` must gain a `trackID string` parameter so
   the updater knows which track entry to modify. Confirm the chosen resolution in the proof bundle.

2. [mechanical] §2/"Risks" — `MergeTrackFn` scope leaves the gate bypass undocumented
   What I observed: The spec Risk says "the auto-trigger can call the underlying merge logic
   directly (bypassing the CLI gate check); the CLI gate in S05 is a wrapper, not the only path."
   The design's `MergeTrackFn` is a bare `git merge --no-ff <track-branch> --no-edit`. The design
   does not state which S05 gates are intentionally bypassed vs. preserved. S05 defines three
   gates: (1) verified-check (all slices verified before merge), (2) invariant-4 classifier
   (conflict detection), (3) `index.md` state update to "merged". The bypass of (1) is safe in
   the automated path because the router only returns "merge-track" after all slices are verified
   — the router IS the verified-check. The bypass of (2) is a downgrade in diagnostics: a
   conflict still causes `git merge` to fail (→ `TrackFail`), but the error message won't name
   which files conflict. The bypass of (3) is the root cause of Pin 1 above. None of this is
   stated in the design.
   What to ask the implementer: Add a comment block in `finishTrack` (near the `MergeTrackFn`
   call site) stating: which S05 gates are bypassed and why each bypass is intentional or
   acceptable. Specifically: "(1) verified-check: satisfied by router — router only emits
   merge-track after all slices verified; (2) invariant-4 classifier: bare merge still fails on
   conflict → TrackFail, lower diagnostic quality than S05; (3) index.md state update: [either
   'handled by MergeTrackFn' or 'not performed — see Pin 1 resolution']." This is a documentation
   fix, not a behaviour change — the implementer adds the comment and the Verifier checks AC3.

## Summary

Pins: 2 total — 2 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: Pin 1 (deadlock under any depends_on release — if `waitForDependencies` is
retained without oracle state update, AC4 "no deadlock" will fail the integration test)

## Smaller flags (not pins, worth one-line acknowledgement)

(a) **S06 and S07 also own `worker.go`/`parallel.go`.** S06-invariant2-enforcement plans
`internal/run/parallel.go`; S07-pause-resume-committed plans `internal/scheduler/worker.go`.
Both are `planned` and come after S04 in T1's sequential slice order. Serial execution resolves
merge ordering automatically (second-lander re-runs the shared test). No action needed now;
S06/S07 implementers should note S04's additions to `WorkerOptions`.

(b) **`pauseSet` package-level var appears unused.** `worker.go:54-61` defines `pauseSet` but
it is not referenced outside its declaration. The design proposes changing the semantics of
`"merge-track"` in the `case` statement directly, which is correct, but `pauseSet` is now stale
dead code. Removing it is out of S04 scope (and shouldn't be done silently) — file this as a
cleanup issue if it matters.

## Suggested acknowledgement reply

<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is sound. Phase-barrier + auto-merge approach is correct. 2 mechanical pins to apply
inline before writing code:

1. **Drop `waitForDependencies` OR update `index.md` in `MergeTrackFn`.** The oracle reads
   `state: merged` from `index.md` on `release-wt`. A bare `git merge --no-ff` does not update
   `index.md`, so `waitForDependencies` polls forever → deadlock. Recommended resolution (a): drop
   `waitForDependencies` and add a comment: "Phase barrier in RunParallel (wg.Wait per phase)
   enforces depends_on ordering — no polling needed." Update AC traceability: AC1/AC4 → phase
   barrier. If you choose (b) instead: extend `MergeTrackFn` to accept `trackID` and atomically
   update `index.md` state to "merged" on `release-wt` before returning.

2. **Document the S05 gate bypass in a comment near `MergeTrackFn`.** State explicitly: (1)
   verified-check is satisfied by the router (emits merge-track only after all slices verified);
   (2) invariant-4 conflict detection is bypassed — bare merge still fails on conflict → TrackFail
   (acceptable downgrade); (3) index.md update: state whichever resolution from Pin 1 applies.

Flags (not pins): (a) S06/S07 own the same files — serial T1 execution handles ordering,
S06/S07 implementers should note new `WorkerOptions` fields; (b) `pauseSet` is now dead code,
low priority cleanup.

§2 decisions (injection points via WorkerOptions, auto-merge in finishTrack, backward-compatible
`case "merge-track":`) acknowledged — all correctly Type-2.

Address pins 1–2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both pins are apply-inline mechanical fixes (drop/replace redundant poll loop; add doc comment on bypass scope); phase barrier correctness is already proven; Verifier checks AC1/AC4 in the integration test.
-->
