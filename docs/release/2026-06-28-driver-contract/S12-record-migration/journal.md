# Journal — S12-record-migration

## 2026-07-11 — Implementer session (design_review → in_progress → implemented)

### Session start / gate
- Coach PROCEED ack committed on this branch (`captain-proceed.md`, decided by Brad):
  both CRITICAL escalate pins ratified. Design-review gate satisfied → proceeded.
- BLOCKED guard: `verification.result` was `pending` (not blocked) → clear.
- Track worktree already materialised; predecessors S11 + S15 both `verified`
  (sequential gate clear). S12 is the FINAL T7 slice, so it migrates S15's records
  too and removes the shim LAST (nothing authors a retired-vocab record after it).

### Decisions (also in status.json design_decisions + the landing commit body)
1. **AC-07 / sworn#95 (Type-1, Coach-ratified — captain-proceed.md pin 1).** The
   migration MAPS each retired AC `type` → canonical `ears_pattern`
   (unwanted→unwanted-behaviour etc.), drops `ears_keyword`, and repoints
   `internal/ears/ears.go` (`classifySpecJSON` → `patternFromEARSPattern`, new
   `spec.AC.EARSPattern` field). Without the coupled reader change, stripping
   `ears_keyword` for strict v0.10.0 would silently collapse every non-ubiquitous
   AC to Ubiquitous. Homed here (not re-opening verified S11) so code + data land
   atomically.
2. **Whitelist board projection (Type-2, review pin 4).** board.json reshaped by
   whitelist to `{$schema, release, tracks:{id,slices,depends_on}}`, and the
   `release` object itself whitelisted to `{name,target_version,integration_branch,
   vertical_trace}`. This dropped a stray `release.worktree` object on render-drift
   that a blacklist shim would have missed (caught by the conformance test — see
   below). schema_version stripped from spec.json/board.json; status.json keeps it.
3. **Records-conformance Go test (Type-2, review pin 3).** `internal/baton/
   records_conformance_test.go` is the AC-03/AC-06 sweep (no CLI runs
   baton.ValidateSchema over on-disk records). One file beyond declared
   touchpoints, Captain-acknowledged. Doubles as durable CI regression + the AC-07
   real-data classification assertion.
4. **AC-02 zero-match guard (Coach pin 2).** Zero `"quadrant":"feature"` records
   exist in JSON; the script's postconditions assert chore/epic/feature = 0.

### Trade-offs / notes
- The migration is `scripts/migrate-records.sh <docs/release>` — jq-only, portable
  to `~/projects/fired`, idempotent (write-only-if-changed; status.json rewritten
  only when its quadrant is chore/epic to avoid churning grind/puzzle records).
- Script scope = releases containing ≥1 spec.json (the five spec-v1-era); this
  excludes legacy markdown-era releases by construction (Coach 2026-07-10). Verified
  all chore/epic quadrants live in the five, so AC-01's all-`docs/release/` grep
  holds.
- The conformance test caught a real defect on the first run: render-drift's
  `release` object carried a stray `worktree` — fixed by whitelisting the release
  object, then re-migrated + re-verified.

### R-01 / review pin 5 — in-flight tracks holding diverged record copies
This migration lands on the integration line (via the T7 track branch). In-flight
releases with live track worktrees hold PRE-migration copies of their own records
and will read "behind" until they forward-MERGE (never cp-files) the migrated base
— the 2026-06-28 replan-propagation lesson. Named so the next session isn't
surprised:
- `2026-06-30-sworn-operational-readiness` — tracks T1..T5 (worktrees present)
- `2026-07-01-release-hygiene` — track T1
- `2026-07-01-render-drift-reconciliation` — tracks T1..T5
- `2026-07-01-loop-cli-ux` — no live track worktrees at this time
Propagation is via `/implement-slice` Step-0 forward-merge self-heal only; this
slice made NO cross-worktree edits (out_of_scope, Rule 11).

### Verification / gates run (implementer, live repo state)
- Full `go test -count=1 -timeout 300s ./...` → exit 0, 47 packages ok (the
  shim-removal fixture gate — no fixture fed a chore/epic record through the now
  strict state.Read).
- gofmt -l clean, go vet clean, newline-eating grep clean on all changed .go.
- `sworn designfit 2026-06-28-driver-contract` PASS; `sworn board --release ...`
  exit 0; 0 `baton.Normalise` references (sworn#90 fully closed).
- AC-07 reachability: `sworn lint ac` over migrated records = ubiquitous 41 /
  event-driven 29 / state-driven 1 / unwanted-behaviour 11 (NOT all-Ubiquitous).

### Deferrals
None. No out-of-scope work absorbed. The follow-up of running the script in
`~/projects/fired` is explicitly out of scope (spec out_of_scope[4]) and executed
in that repo after this release ships.

### First-pass known false-negatives (release-verify.sh)
`release-verify.sh` reports `spec.md missing` and `proof.md missing`. Both are
known false-negatives for spec-v1 slices: every slice in this release uses
`spec.json` (0 spec.md repo-wide), and the verified siblings S11 + S15 carry
neither spec.md nor proof.md and passed verification. The canonical bundle is
`proof.json` (valid against proof-v1). The `PLAYWRIGHT_OPTIN unbound variable`
line is a script-env quirk (run with `PLAYWRIGHT_OPTIN=0`), not a slice failure.

### Terminal state
`implemented`. Proof bundle: `proof.json`. Ready for fresh-context verification.

## Verifier verdicts received

### 2026-07-11 — FAIL (fresh-context verifier, artefact-only)

Verified against track/2026-06-28-driver-contract/T7-baton-revendor @ 4dbbedc
(start_commit 71a2954). Re-ran the full `go test -count=1 -timeout 300s ./...`
(exit 0, 47 pkgs ok) and every AC test independently.

PASS on: AC-01 (grep chore/epic = 0), AC-02 (feature = 0), AC-03
(TestRecordsConformance_SpecV1Era — 33 spec.json strict-v0.10.0 + in_scope/
out_of_scope present; actual `"schema_version":` key only in the 2 excluded
legacy boards), AC-04 (TestReadRejectsRetiredQuadrant; internal/baton/normalise.go
deleted, 0 live baton.Normalise call sites — shim gone wholesale), AC-06
(5 board.json pure-plan {$schema,release,tracks}), AC-07 (type->ears_pattern
mapped, ears.go reads EARSPattern, TestRecordsConformance_EARSClassificationPreserved
+ `sworn lint ac` = 82 ACs non-all-Ubiquitous). Migration script committed,
re-runnable, idempotent (diff -rq copy-vs-worktree IDENTICAL after re-run).
Legacy releases untouched (0 files). gofmt/vet clean.

FAIL on AC-05:
1. `sworn doctor` (the AC-05 fail-closed render-drift guard) exits 1 and flags
   `render drift (2026-06-28-driver-contract) — committed index.md does not match
   render(board.json)`. driver-contract is one of the five spec-v1-era releases
   AC-05 requires the guard to pass for. The committed index.md renders the S12
   row state as `in_progress`; render yields `implemented` (single-line drift,
   quadrant grind correct). The other four of the five render clean.
2. proof.json reachability (l.136) + delivered[AC-05] (l.156-157) claim
   `sworn doctor render-drift flags only the 2 excluded legacy releases, not the
   five` — false against live state; doctor flags the 2 legacy PLUS driver-contract.

Required: re-render 2026-06-28-driver-contract/index.md so the committed file
matches render(board.json) and commit it in this slice; re-run `sworn doctor` and
confirm none of the five are flagged (only the 2 excluded legacy remain); correct
the proof.json AC-05 evidence. Legal in-spec implementer fix — FAIL, not BLOCKED.
Board.json left unchanged (pure-plan carries no slice state); index.md left
un-re-rendered so the drift stands as the AC-05 evidence for the next round.

## 2026-07-11 — Implementer session (round 2: failed_verification → implemented)

Re-entered from `failed_verification` to address EXACTLY the two verifier
violations. `start_commit` unchanged (71a2954). Reset `verification.result`
fail → pending on re-entry (the FAIL verdict history stays above).

### Root cause (both violations share one)
The first pass rendered `docs/release/2026-06-28-driver-contract/index.md` while
`status.json` still read `in_progress`, then flipped the slice to `implemented`
WITHOUT re-rendering. `checkRenderDrift` (cmd/sworn/doctor.go) compares the
committed index.md against `board.Render`, whose Slices-table `State` column is
read from the on-disk `status.json` (`readSliceRecord` → `os.ReadFile`). So the
one S12 cell drifted (`in_progress` committed vs `implemented` rendered). The
Tracks-table state is git-ref-derived (`DeriveTrackState` → `in_progress` for
T7, not an ancestor of release-wt), stable across my commits, so it was NOT part
of the drift — the diagnostic render diff showed a single changed line.

### Fix (legal, in-spec — FAIL not BLOCKED)
1. **AC-05 / Violation 1.** Set `status.json` → `implemented`, then
   `sworn render 2026-06-28-driver-contract` (freshly-built binary). The ONLY
   index.md change is the S12 State cell `in_progress` → `implemented` (quadrant
   `grind` already correct). Live `sworn doctor` render-drift now reports
   `2 of 7` — flagging ONLY the two excluded legacy releases
   (2026-06-19-safe-parallelism: missing S01 spec.json; 2026-06-27-conformance-
   foundation: bare-string board.json). None of the five spec-v1-era releases
   drift. Render is idempotent (index.md md5 96cf8795 stable across a 2nd render).
2. **Rule 6 / Violation 2.** Corrected proof.json `reachability.evidence` and
   `delivered[AC-05]` to the live `sworn doctor` result (2 legacy only, not the
   five) instead of the stale recalled claim; added journal.md to files_changed;
   updated the board test_result ref to S12 `implemented`.

### Live evidence (this session, freshly-built binary)
- Full `go test -count=1 -timeout 300s ./...` → exit 0, 47 packages ok, 0 FAIL/
  panic (doc-only remediation; no Go delta).
- `bash scripts/migrate-records.sh docs/release` → idempotent no-op (processed 5
  spec-v1-era, skipped legacy, "all postconditions satisfied"); git clean after.
- `sworn lint ac 2026-06-28-driver-contract` → 82 ACs well-formed EARS,
  Violations: none; ubiquitous 41 / event-driven 29 / state-driven 1 /
  unwanted-behaviour 11 (NOT all-Ubiquitous — AC-07/sworn#95 preserved).
- `sworn designfit 2026-06-28-driver-contract` → PASS, 15 slices, all gates clear.
- `sworn render` on driver-contract idempotent (md5 stable).

### Out of scope (NOT touched — Rule 11 / role boundary)
`sworn doctor` Group 2b still reports status-timestamp findings. Almost all are
in the two excluded legacy releases (accepted, Coach 2026-07-10). One is
`2026-06-28-driver-contract/S08-honest-cost-telemetry` (last_updated_at
2026-07-11T14:46:03Z reads ~future against this session's wall clock) — a
different, already-`verified` slice and a clock-skew artefact, not an S12 record
and not part of either violation. Editing another slice's record would be
scope-creep; left untouched. The AC-05 requirement is the render-drift guard,
which is clean for all five.

### Terminal state
`implemented`. Proof bundle: `proof.json` (AC-05 evidence regenerated from live
repo state). Ready for fresh-context verification.
