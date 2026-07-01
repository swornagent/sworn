# Proof bundle — 2026-06-28 deepseek-eval replan resume + stale-branch cleanup

Resumes the two REMAINING items from `docs/captures/2026-06-28-session-handoff.md`.

## Scope

Clear the deepseek eval's S12 drift-gate BLOCK by replanning T3/T7 to depend_on
T2 (the recurring `oai.go` shared-surface fix), forward-merge T2's base into T3,
let the running loop resume T3; and delete the 35 stale eval `release-wt/*` /
`track/*` branches in the main repo.

## Files changed

### deepseek clone (`~/sworn-eval-coach-deepseek`, isolated — local bare origin only)
- `release-wt/2026-06-27-conformance-foundation` @ **a061677** — `index.md`:
  T3 & T7 `depends_on: T2-model-layer`; `oai.go` declared DOCUMENTED SHARED
  T2/T3/T7 (frontmatter + touchpoint matrix + cross-slice note). No YAML inline
  comments (the parse trap).
- `track/.../T3-agentic-verifier` @ **befd019** — S12 `status.json`:
  `verification.result` blocked → pending, violations emptied, verdict_at nulled.
- `track/.../T3-agentic-verifier` @ **dfe5514** — merge of release-wt into T3.
  Conflicts resolved: `internal/model/oai.go` (combined T2's `Capabilities()` +
  T3's `RunFirstPass` rename, fixed a `fulfilling//` comment glitch);
  `index.md` (accurate un-fused Slices table + reconstructed Recent-activity log).

### main repo (`~/projects/sworn`) — no commits; branch/worktree deletions only
- 35 stale branches deleted (34 merged-into-`release/v0.1.0` + 1 clearly-eval-only T21).
- 35 worktrees under `sworn-worktrees/` removed; `git worktree prune` run.

## Test results

- `go build ./...` in the T3 worktree post-merge → **exit 0** (oai.go combine compiles).
- `go test ./internal/verify/...` (S12's declared test_commands) → **ok 0.015s**.
- Oracle parse (`release-board-status.sh --json`) → **no corruption warnings**;
  T3 & T7 `dependsOn=["T2-model-layer"]`; S12 `state=implemented actionable=true`.
- Main repo final state: `release-wt/* + track/*` remaining = **0**; worktrees = **1** (main).

## Reachability artefact

The **running** deepseek coach loop (singleton coordinator PID 2988660; ~21 tick
procs) auto-un-parked T3 after the BLOCK clear and dispatched a fresh-context
verifier. Live `loop.log` 13:52:10:
> "State is `implemented`, verification is `pending` — short-circuit does not
> apply. Proceeding. Verifying inside track worktree ... — track already synced
> to `release-wt`."
This is end-to-end proof: S12 cleared (Task 2), drift-gate satisfied by the merge
(Task 3), and the loop carrying T3 forward on its own (Task 4) — through the real
oracle/scheduler integration point, not a unit stub.

## Delivered

- **Replan** — T3/T7 depend_on T2; oai.go shared declared. Evidence: a061677;
  oracle reports `dependsOn=["T2-model-layer"]`.
- **S12 BLOCK cleared** — befd019; oracle reports `actionable=true`.
- **Forward-merge** — dfe5514; `go build` + verify tests green; oai.go combine correct.
- **Loop resumed** — already running (singleton guard); auto-un-parked T3 →
  verifier dispatched. loop.log 13:52:10.
- **Branch cleanup** — 35/35 in-scope branches + worktrees deleted; 0 remain.

## Not delivered (surfaced, not silently dropped)

- **S12 final verdict** — in flight; the loop owns it (the implementer/this
  session must NOT certify — Baton Rule 7). Tracked: oracle `actionable=true`,
  worker dispatching. No action required; the loop will transition it.
- **Out-of-scope leftover branches NOT deleted** (need a human call, per the
  agreed scope of "35 release-wt/track" only): likely cruft — `T3-commercial`,
  `release-2026-06-19-safe-parallelism-T3-commercial`, `-T8-memory`,
  `sworn/hello`, `sworn/test*`, `sworn/verify-state-writes-to-repo-root`,
  `sworn/change-the-phrase-...`, `wip/cli-styling-reference`; **possibly real
  work** — `fix/centralise-baton-version`, `refactor/baton-vendor-paths`.
  Why deferred: outside the verified set the user approved. Tracking: this bundle.
  Acknowledgement: surfaced to Brad in the session summary.

## Divergence from plan

- The handoff said the deepseek loop was **paused**; it was actually **running**
  (resumed since the handoff was written) with T3 **parked** on a `REPLAN:`
  verdict. So "resume the loop" (Task 4) needed no manual restart — the singleton
  guard would refuse a second loop, and the running coordinator auto-un-parked T3
  once the BLOCK was cleared (route_for S12: coach_decision → verify). Net effect
  identical to the handoff's intent; mechanism differed.
- T7 was **not yet merged** into release-wt at resume time (handoff implied
  "T7/S25 already resolved earlier"); harmless — T7 now depends_on T2 (satisfied,
  T2 merged) and the loop is actively driving T7/S25–S26.
