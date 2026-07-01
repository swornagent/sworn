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

## Verifier verdicts received

### 2026-07-01T03:46:52Z — PASS (fresh-context Verifier, Rule 7)

Verified inside track worktree
`sworn-worktrees/release-2026-06-30-sworn-operational-readiness-T3-consumer-repo-hygiene`
against HEAD `488a2bd` (drift vs release-wt = 0, no forward-merge needed).
Fresh session, artefact-only inputs (spec.json / proof.json / status.json +
live repo state); no implementer transcript loaded.

All six mechanical gates PASS:

1. **User-reachable outcome** — `db.Open` is the real integration point and is
   wired from production surfaces (`internal/run/run.go:145`,
   `internal/supervisor/supervisor.go:280`, `cmd/sworn/run.go:222`,
   `internal/tui/concurrent.go`), not a test fixture. When sworn runs against a
   repo, `db.Open` materialises `.sworn/` and stamps `.sworn/.gitignore` = `*`.
2. **Touchpoints match** — code diff is exactly `internal/db/db.go` +
   `internal/db/db_test.go` (the two declared touchpoints); remaining changed
   files are this slice's own proof-bundle docs. No unrelated churn.
3. **Tests exercise the integration point** — the four AC tests drive `db.Open`,
   not a leaf. Re-ran `go test ./internal/db/...`: 5/5 PASS
   (`TestSelfIgnoreWritten` / `…NotOverwritten` / `…HidesSwornDir` /
   `…BestEffort` + `TestOpenCreatesDir`). `go build ./...`, `go vet`, `gofmt -l`
   all clean (exit 0).
4. **Reachability artefact** — `TestSelfIgnoreHidesSwornDir` git-inits a temp
   repo, opens the DB via `db.Open`, asserts `.sworn/` absent from
   `git status --porcelain`. Names the user gesture, matches the spec outcome,
   PASSES in my re-run.
5. **No dark code** — grep for TODO/FIXME/deferred/placeholder/XXX/HACK/later
   over the changed code files: none. No stubs or hardcoded happy-path returns.
6. **Claimed scope = implemented scope** — every `delivered` item (AC-01…AC-05)
   maps to a real, working evidence reference; no delivered item outside the
   spec's acceptance criteria.

Independent AC-01 check (spec-demanded): confirmed in production code that BOTH
the run DB (`cmd/sworn/run.go:222` → `.sworn/sworn.db`) and the supervisor DB
(`internal/supervisor/supervisor.go:279-280` → `.sworn/supervisor-<release>.db`)
route through `db.Open`, so the single `filepath.Base(dir) == DefaultDir`-gated
`writeSelfIgnore` covers both. The `*` ignore covers the whole `.sworn/` dir, so
any other writer under it (e.g. the config-driven memory index, which is not
`.sworn/`-scoped and out of AC-01's named scope) is moot for the user outcome.

LLM gates 3b (ac-satisfaction) / 4b (semantic-coverage) skipped —
`SWORN_ANTHROPIC_API_KEY` unset (non-blocking per verifier contract);
substituted with the manual per-AC walk above. Design gate (Rule 9) auto-passes:
non-UI Go slice.

Board: no change — `board-v1` track-state enum is `planned|in_progress|merged`
(no `verified` track state); T3 stays `in_progress` until `/merge-track`. The
verified transition is recorded in `status.json`.

Verdict: **PASS**. Next: `/merge-track T3-consumer-repo-hygiene` (S03 is the sole
slice in T3 → track complete).
