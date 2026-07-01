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

## 2026-07-01 — in_progress → implemented (Implementer)

Landed the design as specified — no divergence from design.md. `db.Open` now
computes `dir := filepath.Dir(dbPath)` once, and after a successful `MkdirAll`
calls `writeSelfIgnore(dir)` when `filepath.Base(dir) == DefaultDir`.
`writeSelfIgnore` is a small unexported helper returning an error that `Open`
deliberately ignores (best-effort). Four AC tests added
(`TestSelfIgnoreWritten` / `…NotOverwritten` / `…HidesSwornDir` / `…BestEffort`).

Trade-offs / notes:
- **gofmt fix at the touchpoint.** `internal/db/db.go` carried a pre-existing
  gofmt violation (missing blank line after the `schema` var block) right at my
  edit boundary. Ran `gofmt -w` on the file I was already editing so it stays
  gofmt-clean; this is within my declared touchpoint, not scope creep. The
  older `} // Enable foreign keys.` quirk on the WAL/foreign-keys line is
  gofmt-legal and left untouched.
- **AC-04 test uses a directory at the `.gitignore` path** (per design flag) so
  the write fails deterministically while `Open` still succeeds — proves
  best-effort independently of AC-02's existing-file case.
- **New tests build paths under `.sworn/`** so the `filepath.Base==DefaultDir`
  gate fires; the existing `tempDB` helper opens under a random temp dir (base
  != `.sworn`), so pre-existing tests are unaffected by the new behaviour.

Verification-gate divergence (Rule 2, surfaced): the installed `sworn`
binary's `verify` is the model-backed `--spec/--diff` judge and needs
`SWORN_ANTHROPIC_API_KEY`, which is unset here — so the model-backed first-pass
could not run. Ran the deterministic `~/.claude/bin/release-verify.sh` instead
(all structural checks PASS; only FAIL was the expected `in_progress` state
check, cleared by this transition). Canonical model verification is the
fresh-context `/verify-slice` (Rule 7). Full detail in proof.json/proof.md
"Divergence".
