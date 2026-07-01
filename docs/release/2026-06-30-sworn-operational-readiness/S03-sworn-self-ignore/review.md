# Captain review — S03-sworn-self-ignore
Date: 2026-07-01
Design commit: 75ecef0770904045567635de8b2cb567d0b5abf9

## Pins

1. [memory-cited] §Rationale/purpose — self-ignore directly serves the dominant loop-failure mode.
   What I observed: the spec rationale and design both frame `.sworn/` showing as `?? .sworn/` in git status as "load-bearing, not cosmetic" for an unattended run. This is the exact domain [[project_coach_loop_worktree_hygiene]] codifies: a dirty worktree is the dominant coach-loop failure mode.
   What to ask the implementer: acknowledge the citation — the slice's purpose is squarely on-memory; no conflict. Confirmation only.
   Citation: [[project_coach_loop_worktree_hygiene]]

2. [mechanical] §Design-level risks (bullet 1) — gate on `filepath.Base(dir) == DefaultDir` is correct; audit already performed.
   What I observed: the design flags for the reviewer whether gating the write on `filepath.Base(dir) == ".sworn"` (vs writing unconditionally into `filepath.Dir(dbPath)`) is desired, resting on the inference "every real caller passes a `.sworn/…` path, so the gate never suppresses a wanted write."
   What to ask the implementer: nothing to change — I audited all six `db.Open` callers (`cmd/sworn/run.go:222`, `cmd/sworn/telemetry.go:368`, `internal/run/run.go:145`, `internal/supervisor/supervisor.go:280`, `internal/tui/concurrent.go` ×3). Every path resolves under `.sworn/` (telemetry: `wd+"/.sworn/sworn.db"`; supervisor: `.sworn/supervisor-<release>.db`). The gate fires for every wanted write. Proceed with the gated design as written; apply inline.

3. [mechanical] §Stakes classification — Type-2 classification lives in design.md prose but not in status.json `design_decisions`.
   What I observed: design.md classifies all three decisions Type-2 (reversible, narrow, no architecturally-significant surface) — which I agree with: confined to `internal/db`, no public API change. But S03's `status.json` carries no `design_decisions` field for a machine-readable design-fit-gate input.
   What to ask the implementer: non-gating — verified siblings S04/S05 reached `verified` without the field too, so this matches project convention (design.md prose is treated as sufficient). Either confirm that convention or populate `design_decisions` in status.json for the audit trail. Not a blocker; apply inline if populating.

## Summary

Pins: 3 total — 2 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): none — no pin would cause the slice to ship broken if unaddressed.

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) AC-04 best-effort test via a pre-existing **directory** at the `.gitignore` path (so the write fails deterministically while `Open` still succeeds) is a sound, deterministic way to prove best-effort independently of AC-02's existing-file case — acknowledge.
- (b) `~/.sworn/.gitignore` courtesy write: if any opener ever targets `~/.sworn/…`, a harmless ignore file lands there. Courtesy-only, no repo there normally — acknowledge, no action.
- (c) Positive: because `os.MkdirAll` returns nil whether it created `.sworn/` or found it pre-existing, `writeSelfIgnore` runs on every `Open` and `O_EXCL` makes it a no-op once present — this retro-fixes a pre-existing `.sworn/` from an older sworn that never had the ignore. Good.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Tight, well-scoped chore — single chokepoint correctly identified, both DBs provably covered, O_EXCL collapses idempotency+best-effort into one syscall. Clean PROCEED. 3 pins (all apply-inline) + 3 flags:

1. **Worktree-hygiene citation.** Your purpose is squarely on [[project_coach_loop_worktree_hygiene]] (dirty worktree = dominant loop-failure mode). Acknowledged — no conflict.
2. **Gate correctness — already audited, proceed as written.** I confirmed all six `db.Open` callers resolve under `.sworn/` (run/telemetry/supervisor/tui), so gating on `filepath.Base(dir)==DefaultDir` never suppresses a wanted write. Keep the gate.
3. **design_decisions field.** Type-2 classification is in design.md prose but not status.json. Non-gating (S04/S05 verified without it). Either follow that convention or populate `design_decisions` when you land status.json — your call, apply inline.

Flags (not pins): (a) AC-04 test-via-directory is sound, keep it; (b) `~/.sworn/.gitignore` courtesy write is harmless, no action; (c) writeSelfIgnore running on every Open (MkdirAll nil on pre-existing dir) is a feature — it retro-fixes older `.sworn/` dirs.

§2 decisions 1 (O_EXCL helper), 2 (base==DefaultDir gate — audited), 3 (`*\n` content) acknowledged. §6 items (gate correctness, AC-04 testability, ~/.sworn) all addressed — no Coach decision needed.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 3 pins are apply-inline confirmations (memory citation + a gate audit I already completed + a non-gating convention check); none changes the design or needs Coach judgement. Touchpoints are a best-effort, idempotent, non-destructive .gitignore write — no auth/payments/PII/migration/destructive domain.
-->
