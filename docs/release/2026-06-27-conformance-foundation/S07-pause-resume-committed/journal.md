# Journal — S07-pause-resume-committed

## 2026-06-28 — Replan (Planner/Coach, revision mode)

**Trigger:** Captain design review returned `DECISION: NEEDS_COACH` on design commit 926d66b. Two critical pins required Coach/spec authority before code was safe.

**Diagnosis (verified against live code):**
- Pin 2 (confirmed): the original spec premise — "`findFirstNonTerminal` reads the working-tree copy" — is false. `findFirstNonTerminal` (worker.go:536) returns `slices[0]` unconditionally and reads no state. Routing already flows through committed git refs via the router + `oracle.ReadSliceStatus`. The described "dirty working-tree re-runs the wrong slice" bug is not in the frontier-selection path.
- Pin 1 (confirmed): original AC2 ("skip past `implemented`") regresses forward-only resume. The router walk (`routeVerified`, router.go:271) never returns; skipping an `implemented` slice abandons it instead of re-verifying it. The router already treats `implemented` as non-terminal (`routeImplemented`, router.go:251).
- Pin 3 (confirmed): terminal-set is defined twice in the router (router.go:307, :393) as `{verified, shipped, deferred}`; the original design introduced a third, divergent set in the scheduler.
- Untracked finding (confirmed): worker.go:232 — the all-terminal `return finishTrack(...)` is fused onto its comment line and is commented out. Dead today (seed never returns ""), but this slice's committed-read change makes "" reachable → a fully-terminal track on resume would fail to merge.

**Coach decision (Brad, 2026-06-28): replan properly** (chosen over defer / narrow-fix). Re-anchor the spec to the real behaviour: seed from committed state via the oracle; treat `implemented` as non-terminal (DD-1); unify the terminal-set in one exported `router.IsTerminal` helper (DD-2); fix the worker.go:232 fused-line bug (AC4). Original AC2 replaced.

**Artefacts updated:** spec.md (re-scoped, EARS ACs AC1-AC6, Coach decision section), status.json (design_decisions DD-1/2/3; planned_files += internal/router/router.go; test_commands updated; verification.result reset pending; state planned). Design gate artefacts (design.md/review.md/captain-proceed.md) stripped so the Design TL;DR gate re-fires fresh against the corrected spec.

**Next:** loop re-dispatches `/implement-slice S07` against the corrected spec.
