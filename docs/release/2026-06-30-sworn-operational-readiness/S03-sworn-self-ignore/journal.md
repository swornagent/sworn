# Journal — S03-sworn-self-ignore

## 2026-07-01 — design_review → in_progress (Implementer)

Design gate satisfied: `review.md` carries `DECISION: PROCEED` (CAPTAIN-VERDICT,
non-constitutional) and the Coach delivered the acknowledgement reply. Proceeding
to implementation. Slice is the sole slice in track `T3-consumer-repo-hygiene`
(no sequential-order gate); `verification.result` was `pending` (Step 0b: not
blocked).

### Design-review pins — all apply-inline, acknowledged

1. **[memory-cited] Worktree-hygiene citation.** Acknowledged — the slice's
   purpose (`.sworn/` showing as `?? .sworn/` in git status is load-bearing for
   an unattended run) is squarely on `project_coach_loop_worktree_hygiene` (a
   dirty worktree is the dominant coach-loop failure mode). No conflict.
2. **[mechanical] Gate on `filepath.Base(dir) == DefaultDir`.** Keeping the gate
   as designed. The Captain audited all six `db.Open` callers
   (`cmd/sworn/run.go`, `cmd/sworn/telemetry.go`, `internal/run/run.go`,
   `internal/supervisor/supervisor.go`, `internal/tui/concurrent.go` ×3) and
   confirmed every wanted write resolves under `.sworn/`, so the gate never
   suppresses a wanted write.
3. **[mechanical] `design_decisions` in status.json.** Non-gating. Verified
   siblings S04/S05 reached `verified` with design decisions recorded as
   design.md prose only. Following that ratified project convention — design.md
   §"Stakes classification (Rule 9)" is the design-decision record (all three
   choices classified Type-2). Not populating a `status.json.design_decisions`
   field, to avoid introducing a schema shape the sibling-verified slices did
   not carry. `confirmed_by_implementer` set to `true` on the effort/complexity
   block.

### Flags (acknowledged, no action)

- (a) AC-04 best-effort proven via a pre-existing **directory** at the
  `.gitignore` path (deterministic write failure while `Open` still succeeds).
- (b) `~/.sworn/.gitignore` courtesy write is harmless (no repo there normally).
- (c) `writeSelfIgnore` runs on every `Open` (MkdirAll returns nil on a
  pre-existing dir) and `O_EXCL` no-ops once present — retro-fixes older
  `.sworn/` dirs. Intended.

### Approach (from design.md)

Single chokepoint: `db.Open` in `internal/db/db.go`, immediately after the
`os.MkdirAll` that materialises the DB's parent dir. Best-effort
`writeSelfIgnore(dir)` using `O_CREATE|O_EXCL|O_WRONLY`, gated on
`filepath.Base(dir) == DefaultDir`. Content `"*\n"`. Both run DB and supervisor
DB route through `db.Open`, so one write covers both.
