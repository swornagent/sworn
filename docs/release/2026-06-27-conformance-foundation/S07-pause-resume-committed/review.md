# Captain review — S07-pause-resume-committed
Date: 2026-06-28
Design commit: 926d66b5105d5b69dc059eba72a8983a172cc2a6

## Pins

1. [escalate] §2.2 / spec AC2 — treating `implemented` as terminal causes a forward-only resume **overshoot** (CRITICAL)
   What I observed: design §2.2 defines `isTerminal = {verified, implemented, shipped}` and spec AC2 directs the system to "skip past" an `implemented` slice. But the worker walks the frontier **forward only**: `runTrackRouter` (worker.go:230–277) seeds `currentSlice` from `findFirstNonTerminal` and thereafter only advances via `decision.Target`; `routeVerified` (router.go) iterates slices forward and never returns. `findFirstNonTerminal` is *only* the seed. If the seed skips an `implemented` slice, it lands on a *later* slice and the forward-only router can never return to that implemented slice — yet the router's `routeImplemented` exists precisely to drive `implemented → /verify-slice`. Net effect: a resume where a committed slice is `implemented` (crashed before verification) is **abandoned**, the opposite of this slice's stated goal. The conflict is rooted in spec AC2, so the implementer cannot resolve it without spec authority.
   What to ask the implementer: Coach decides — does AC2's "skip `implemented`" stand (accepting that implemented-but-unverified slices are skipped on resume), or must `implemented` be treated as **non-terminal** for seeding (matching `routeImplemented`)? The latter is almost certainly correct and needs `/replan-release` to correct AC2.

2. [escalate] §Approach / spec premise — the routing path already reads committed state; `findFirstNonTerminal` reads *neither* working-tree nor committed today (CRITICAL)
   What I observed: the spec frames the defect as "`findFirstNonTerminal` reads the **working-tree** copy." Live `findFirstNonTerminal` (worker.go:536) reads **no** state — it returns `slices[0]` unconditionally (its own comment says so). The routing decision (which slice to run) already flows through `router.Route` → `oracle.ReadSliceStatus`, which reads **committed** git refs (track → release-wt → HEAD, oracle.go), and `routeVerified` walks forward over committed state. The only working-tree `state.Read` calls (internal/run/slice.go) are inside the per-slice implement/verify loop — *not* the frontier-selection path. So the "dirty working-tree re-runs the wrong slice" bug as described is not demonstrably present in the routing path, and seeding from `slices[0]` is already safe-by-construction (never overshoots; the committed router walk corrects it).
   What to ask the implementer: Coach/Planner — confirm the **actual** defect and where it lives before building. Is there a concrete reproduction (a `--resume` that re-ran the wrong slice)? If yes, capture it and re-anchor the spec to the real fault. If no, S07's scope needs re-diagnosis — building it as specified hardens a seed that is already safe and introduces the Pin 1 overshoot.

3. [escalate] §2.2 — terminal-set **drift**: a third, divergent definition of "terminal"
   What I observed: the router already defines terminal twice — router.go:307 and router.go:393 — both as `{verified, shipped, deferred}`. Design §2.2 introduces a third definition in the scheduler as `{verified, implemented, shipped}`: it **drops `deferred`** and **adds `implemented`**. The code comment at worker.go:533–535 explicitly states "the authoritative state machine lives in the router (S58)." A second, divergent terminal definition in the scheduler contradicts that single source of truth, and the `deferred` omission means a `deferred` seed would route to `NextNone` → worker `case "none"` → `finishTrack` (premature track-done).
   What to ask the implementer: do not introduce a divergent set — align with the router's definition and, ideally, centralise one helper consumed by both router and scheduler. The exact correct set depends on the Pin 1 resolution.

4. [mechanical] §2 / status.json — Rule 9 design-fit gate: `status.json` carries no `design_decisions`
   What I observed: `status.json` has no `design_decisions` field, yet the design makes at least three choices (closure-parameter signature; terminal-set semantics; `OracleReader` on `WorkerOptions` + `--resume`). The terminal-set semantics (Pin 1/3) is behaviour-shaping, not a local refactor.
   What to ask the implementer: record `design_decisions` in `status.json` and classify each Type-1/Type-2; the terminal-set semantics needs an explicit classification (and, if Type-1, a recorded Coach decision) before code.

5. [mechanical] §3 / AC3 — the "fallback already handled by the oracle priority chain" claim is an inference, not verified
   What I observed: design (AC traceability, AC3) asserts the track-branch-unreadable fallback to release-wt is "already handled by `Oracle.ReadSliceStatus` priority chain." But `ReadSliceStatus` propagates any error from priority-1 (`if err != nil { return SliceState{}, "", err }`) rather than falling through to priority-2. `readSliceStatusFromRef` returns `("", "", nil)` only when the path is *absent* (CatFileExists false); a **nonexistent track ref** may make `resolvePrefix`/`CatFileExists` *error* (git cat-file on a bad ref), which would propagate and skip the release-wt fallback — breaking AC3.
   What to ask the implementer: grep/read `resolvePrefix` and `CatFileExists` for the nonexistent-ref case. Confirm a missing track branch returns empty (→ falls through to release-wt) rather than an error. If it errors, AC3's fallback does not hold and needs handling in `findFirstNonTerminal` or the oracle.

6. [mechanical] §3 — what observable behaviour does `--resume` actually gate?
   What I observed: design adds `--resume` (requires `--parallel`, else exit 64). But the proposed behavioural change (committed read in `findFirstNonTerminal`) is unconditional once `OracleReader` is wired — it runs on every `--parallel` invocation, not only on resume. Spec in-scope says "on resume, the scheduler calls the updated `findFirstNonTerminal`," but that function is called on every run.
   What to ask the implementer: state what `--resume` changes that a normal `--parallel` run does not. If nothing, either give it real semantics (e.g. only-then consult committed state, or a distinct frontier policy) or document it as a UX affordance with no behavioural delta.

## Summary

Pins: 6 total — 3 [mechanical], 0 [memory-cited], 3 [escalate]
Critical pins: 1 (forward-only overshoot regresses resume for `implemented` slices), 2 (spec premise does not match live code; the described bug is not in the routing path)

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) §2.4's anchor "parallel.go line 183" is accurate — `NewOracleReaderAdapterFromRepo` is constructed at parallel.go:183, wired into `WorkerOptions` at 275/337. Good anchor.
- (b) Naming: spec says `board.Oracle.ReadSliceState`; design says `OracleReaderAdapter.ReadSliceStatus`; the live method is `Oracle.ReadSliceStatus` (interface `OracleReader.ReadSliceStatus`). Design's name is closer; align the spec's name when re-touched.
- (c) `--resume` without `--parallel` → exit 64 is a sound usage-error convention; no concern.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Design is mechanically faithful to the spec, but the spec's diagnosis is suspect and AC2 encodes a resume regression — two escalations need the Coach's call before code is safe. 6 pins + 3 flags:

1. **`implemented` must not be skipped on resume.** The frontier walk is forward-only; `findFirstNonTerminal` only seeds. Skipping an `implemented` slice (AC2 / design §2.2) means the forward-only router never returns to verify it — resume abandons it. Coach resolution: treat `implemented` as NON-terminal for seeding (match `routeImplemented`), via `/replan-release` to correct AC2. Apply that resolution.
2. **Confirm the real defect before building.** `findFirstNonTerminal` reads no state today (returns `slices[0]`); routing is already committed via `router.Route → oracle.ReadSliceStatus`, and the only working-tree reads are in the per-slice loop, not frontier selection. Either anchor S07 to a concrete `--resume` reproduction, or re-diagnose scope. Apply the Coach's re-anchoring.
3. **Don't fork "terminal."** Reuse the router's terminal definition (`{verified, shipped, deferred}`, router.go:307/393) — ideally one centralised helper — rather than a divergent `{verified, implemented, shipped}` set in the scheduler. Final set follows pin 1.
4. **Record `design_decisions` in status.json.** Classify the closure-signature, terminal-set semantics, and wiring choices Type-1/Type-2; the terminal-set semantics is behaviour-shaping — classify it explicitly before code.
5. **Verify the AC3 fallback.** Read `resolvePrefix`/`CatFileExists` for a nonexistent track ref: `ReadSliceStatus` propagates priority-1 errors instead of falling through to release-wt. Confirm a missing branch returns empty, not an error; if it errors, handle it so AC3 holds.
6. **Define `--resume`'s observable effect.** The committed read is unconditional under `--parallel`; state what `--resume` changes, or document it as a no-behaviour-delta affordance.

Flags (not pins): (a) parallel.go:183 anchor is accurate; (b) align the spec/design/live method name on `ReadSliceStatus`; (c) `--resume` without `--parallel` → exit 64 is fine.

§2 decisions: 1 (closure signature) and 4 (parallel.go wiring) acknowledged as clean. Decisions 2 (terminal set) and 3 (`OracleReader`/`--resume` wiring) carry pins 1/3/6. §6: design has no open-questions section — pins 1–2 surface the questions it should have raised.

Address pins 3–6 inline during implementation; apply the Coach's resolution of pins 1–2, then proceed to in_progress.

## UNTRACKED FINDINGS

`gh issue create` failed — this repo has no GitHub remote configured (public-safe, GitHub host absent). The Coach must file the following by hand:

- **Bug: `runTrackRouter`'s all-terminal `finishTrack` is commented out (worker.go:232).** The `return finishTrack(ctx, opts, workRoot, trackID, trackBranch, releaseTrack)` sits on the *same physical line* as the preceding `// All slices already in a terminal state.` comment, so it is commented out and the `if currentSlice == "" {…}` block is empty. When `findFirstNonTerminal` returns `""` (all slices terminal), execution falls through to the `for` loop with `currentSlice == ""`; the router is polled with an empty sliceID, the oracle's `ReadSliceStatus` fails ("no owning track found"), and the track returns `TrackFail` instead of merging via `finishTrack`. Impact: a fully-completed track fails on resume instead of merging. Likely introduced when S04 landed the router loop (3aeaff7) — a newline was lost. Out of S07's stated scope but **exposed/worsened by it**: S07's committed read makes `findFirstNonTerminal` correctly return `""` more often, hitting this dead path. Found during S07 Captain design review.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Spec AC2 directs skipping `implemented` slices, which regresses forward-only resume (pin 1), and the spec's working-tree premise does not match live code where routing is already committed (pin 2) — both need Coach/Planner authority via /replan-release before code is safe.
-->
