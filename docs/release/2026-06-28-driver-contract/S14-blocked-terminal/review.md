# Captain review — S14-blocked-terminal
Date: 2026-07-11
Design commit: 051de7b38d0fc057bd2be1c4eb7290f19bbb498a

## Pins

1. [mechanical] §Approach-4 — `releaseTrack("blocked")` silently records the track as **done**
   What I observed: the design's worker blocked-branch calls `releaseTrack("blocked")`, but `supervisor.Release` (internal/supervisor/supervisor.go:205-207) coerces any state other than `StateDone`/`StateFailed` to `StateDone`. A blocked — unmergeable, replan-required — lane would land in the tracks DB as `done`. (The existing `releaseTrack("paused")` calls share this coercion, but they are not this slice's to fix.)
   What to ask the implementer: call `releaseTrack(supervisor.StateFailed)` in the blocked branch — the blocked/failed distinction already travels via `RecordBlocked`, which is what the report consumes. Do not pass a string `Release` silently rewrites. If you instead extend `Release`'s vocabulary, that is an edit to shared supervisor semantics and must be its own declared decision.

2. [mechanical] §Key-decisions — `design_decisions` absent from status.json
   What I observed: S14's status.json carries no `design_decisions` array; every landed sibling in this release (S01–S08, S13) records one. Rule 9's design-fit gate reads that field.
   What to ask the implementer: at the in_progress transition, record D1–D6 in `status.json.design_decisions`. D1 should cite spec in_scope item 1's explicit "blocked boolean **or verdict enum member**" allowance plus this review's acknowledgement as the human trace; D2–D6 are Type-2 noted defaults with their rejected alternatives as written in design.md.

3. [memory-cited] §D1 — contract-surface choice aligns with the recut release's vocab-binding discipline
   What I observed: D1 (existing `StatusBlocked` enum member + additive `BlockedReason` string, zero-value = today's semantics, no driver emits it this slice) matches the ErrKindAuth-style vocab-binding precedent and the S01 contract-file additive-change discipline (R-02 mitigation). Verified: `ErrKindAuth`/`TerminalErrKind` live in internal/driver/subprocess.go:22-55; `Result` has no `Completed` field, so `Status` is the sole completion carrier and blockedness stays un-inferable from prose by construction.
   What to ask the implementer: nothing — acknowledging confirms the citation.
   Citation: [[project_driver_contract_recut]]

4. [memory-cited] §Approach-3 — reliance on board/router blocked-visibility is now sound
   What I observed: the implement-leg blocked branch leaves `state=in_progress` with `verification.result=blocked`. [[project_oracle_blocked_invisible]] documented the old oracle's blindness to `verification.result` (blocked slices invisible behind `.state`). Verified against live code: the Go oracle derives `Blocked` from `verification.result == "blocked"` regardless of state (internal/board/oracle.go:233) and emits `blocked_reason`/`blocked_owner`; router.Route checks `ss.Blocked` **before** the state switch and routes to `/replan-release` (internal/router/router.go:100-110). `"needs_planner"` is real vocabulary at both ends (internal/verdict/verdict.go:49, internal/board/oracle.go:33).
   What to ask the implementer: nothing — the resume path the design declares unchanged genuinely works for the new write shape.
   Citation: [[project_oracle_blocked_invisible]]

5. [mechanical] §Consequence — terminal driver errors join the BLOCKED report section; confirm as designed
   What I observed: verified — the S07 terminal-driver-error path (internal/run/slice.go:517-521, auth/credits) already returns the `errVerdictBlockedPrefix`-shaped error, so under the worker's sentinel classification these lanes render as BLOCKED with a route-to-`/replan-release` directive, though their remedy is credentials/environment, not a spec replan. Semantically correct (not clearable by re-dispatch); the verbatim reason ("terminal driver error (auth): … check provider credentials") rides into the report and self-explains the true remedy. Note these lanes write no `verification.result=blocked` (the slice.go:521 return precedes any status write), so on a later resume the router still routes them implement — correct for a fixable env failure; BLOCKED-for-this-run vs BLOCKED-persistent diverge here by design.
   What to ask the implementer: accept the widening as designed. Do NOT add a prose-sniffing exemption to carve these out of the BLOCKED section — reason-string matching is the exact anti-pattern this slice removes.

## Summary
Pins: 5 total — 3 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins: 1 (blocked lane recorded as "done" would corrupt the tracks DB signal; fix is unambiguous and apply-inline)

## Smaller flags (not pins, worth one-line acknowledgement)
- (a) `RecordBlocked` reason = "text after the sentinel" will include the " — route: /replan-release (BLOCKED is terminal for this lane)" suffix on implement-leg lanes, so the report renders the directive twice; trim the known suffix when recording (or accept the duplication — verbatim-substring assertions still pass either way).
- (b) After this slice no production driver emits `StatusBlocked` on the implement leg (spec out_of_scope item 4) — the live reachability is the verify-leg BLOCKED verdict and the terminal-driver-error path. Declare this in proof.md so dark-code scanning reads it as spec-sanctioned; also expect scripts/release-verify.sh's known "spec.md missing" false-FAIL on this spec-v1 slice ([[feedback_releaseverify_specmd_false_fail]]) — declare, don't manufacture a spec.md.
- (c) Verified regression-safety of D6: no scheduler test wires a Notifier or asserts `track_failed`, and `TestRunTrack_TerminalDriverErrorHaltsTrack` asserts only TrackFail + dispatch order — the notify-skip and breaker-skip cannot break the protected set. `TestRunParallel_FailureCascade` asserts only a non-nil error mentioning "T1", so the byte-identical-when-no-blocked-lane commitment is stronger than the test requires — keep it anyway (unknown callers may match the format).
- (d) S07 interaction confirmed correct: blocked → `TrackFail` inherits `failCancel()` (dependent later-phase tracks skip via `phaseCtx.Err()`), sibling tracks are not cancelled mid-run (workers run on parent ctx, #33), and the S07 startup resolution sweep (cmd/sworn/run.go:120) is orthogonal — it resolves models before workers spawn and never re-launches lanes.
- (e) Declared skip (Rule 2): the post-PROCEED `sworn llm-check -type design-review` could not run in this session — no model configured (`$SWORN_MODEL` unset, no --model credentials available). Why: environment lacks provider credentials; tracking: recorded here for the Coach; acknowledgement: Coach reads this review. The Verifier (Rule 7) backstops.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Strong design — every cited symbol verified against live code, spec risks R-01/R-02/R-03 mitigations all honoured, AC-06 protected-test set confirmed real and untouched by the plan. 5 pins + 4 flags:

1. **releaseTrack state.** In the worker blocked branch, call `releaseTrack(supervisor.StateFailed)`, not `releaseTrack("blocked")` — `supervisor.Release` coerces unknown states to `StateDone` (supervisor.go:205-207), which would record a blocked lane as done. The blocked/failed distinction travels via `RecordBlocked` only.
2. **Record design decisions.** At the in_progress transition, write D1–D6 into `status.json.design_decisions` (sibling-slice format). D1 cites spec in_scope item 1's "blocked boolean or verdict enum member" allowance + this acknowledgement as the human trace; D2–D6 Type-2 noted defaults.
3. **D1 confirmed.** [[project_driver_contract_recut]] vocab-binding precedent applies; enum + additive `BlockedReason`, zero-value default, no driver emission this slice — proceed exactly as designed.
4. **Resume path confirmed.** The oracle (board/oracle.go:233) and router (router.go:100) key off `verification.result`/`ss.Blocked` independent of state, so the in_progress+blocked write is board-visible and replan-routed. No change needed.
5. **Terminal-error widening accepted.** Auth/credits lanes render in the BLOCKED report section as designed; keep the verbatim reason (it self-explains the credentials remedy) and do not add any reason-string sniffing to carve them out.

Flags (not pins): (a) trim the " — route: /replan-release (…)" suffix from the reason `RecordBlocked` captures, or accept the doubled directive in the report; (b) declare in proof.md that no production driver emits implement-leg `StatusBlocked` yet (spec out_of_scope 4) and expect release-verify.sh's known spec.md false-FAIL on spec-v1 — declare, don't manufacture; (c) after the edit sweep, grep changed .go files for '//' comment lines with code fused on (newline-eating corruption), run gofmt -l + go vet on changed packages; (d) run the FULL `go test -count=1 -timeout 300s ./...` before any state transition — a tightened contract can regress fixtures in other packages.

§2 decisions D1 [memory-cited: project_driver_contract_recut], D2–D6 acknowledged (D3's rejected `TrackBlockedLane` alternative noted and agreed — the enum is taken by invariant-2 and AC-05 requires a distinct report, not a distinct scheduler enum). §6 empty — the design's self-flagged consequence is resolved as pin 5. Confirm `effort_complexity` (low/high, puzzle) at the in_progress transition.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All pins are apply-inline corrections (one critical but unambiguous supervisor-state fix); design verified symbol-by-symbol against live code with no spec deviation and no Coach-authority judgement outstanding.
-->
