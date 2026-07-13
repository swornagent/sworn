# Design TL;DR — S12-record-migration

**Release:** 2026-06-28-driver-contract · **Track:** T7-baton-revendor (FINAL slice) · **State target:** `design_review`

## 1. User outcome (from spec)

Every spec-v1-era release record on the integration branch conforms to the vendored **baton v0.10.0** contract: quadrant `chore`→`quick` and `epic`→`beast`; `in_scope`/`out_of_scope` present on every `spec.json`; each of the five spec-v1-era `board.json` reduced to pure-plan board-v1; the one invalid `feature` quadrant fixed; `index.md` renders refreshed; and then S11's read-path normalise shim removed so the reader enforces the v0.10.0 enum only. The sweep lands as a **committed, re-runnable, idempotent script** so consumer repos (`~/projects/fired`) can migrate their own records. This is the **data half** of sworn#48 (S11 was the code half).

## 2. Live data landscape (surveyed this session, from the T7 worktree)

Base note: this track worktree is cut from `release-wt/2026-06-28-driver-contract`, which is **3 commits behind** integration `release/v0.1.0` — but all 3 are `docs(capture)` handoff commits with **no record-data changes**, so the data landscape here equals the integration tip. Confirmed by re-running the surveys against `release/v0.1.0`.

The **five spec-v1-era releases** (the only ones carrying `spec.json`; legacy markdown-era releases have **0** `spec.json`, so they are naturally excluded from every "every spec.json" AC):

| Release | spec.json | Notes |
|---|---|---|
| 2026-06-28-driver-contract | 15 | this release (incl. S12, S15) |
| 2026-06-30-sworn-operational-readiness | 6 | board carries stray `activity` key |
| 2026-07-01-loop-cli-ux | 3 | board already lacks worktree fields (only `state` on tracks) |
| 2026-07-01-release-hygiene | 2 | board carries `release_worktree_branch` (no path) |
| 2026-07-01-render-drift-reconciliation | 7 | board carries stray `activity` key; S03 is `epic` |

Quadrant distribution across all `docs/release/`: **chore = 38 files**, **epic = 10 files** (48 record files ≈ 24 slices × {spec,status}), grind = 10, puzzle = 8. (The spec's "42 files" estimate is an undercount; the ACs are grep-zero assertions, so the exact count is immaterial.)

**Board shapes vary** (offending top-level / track keys strict board-v1 forbids — allowed top-level is exactly `{$schema, release, tracks}`, allowed track is exactly `{id, slices, depends_on}`):
- top-level strays seen: `schema_version` (all 5), `release_worktree_path`, `release_worktree_branch`, **`activity`** (2 boards).
- track strays seen: `state` (all 5), `worktree_path`, `worktree_branch`.
- slices are **string arrays** already (board-v1 canonical *shape* migration = baton#54, explicitly out of scope) — the reshape preserves `slices` untouched.

## 3. Approach

The unifying invariant: **the migration bakes onto disk exactly what S11's read-path `baton.Normalise` produced in memory** — so removing the shim in the same slice is behaviourally a no-op on already-migrated data. Two deliberate *extensions* beyond the shim's blacklist are required (see pins P-2, P-3).

### 3a. The migration script — `scripts/migrate-records.sh <docs-release-root>`

- **Committed, re-runnable, idempotent**, taking a `docs/release` root as its single argument (so `~/projects/fired` runs the identical tool as follow-up, out of scope here — sworn#48). Version-neutral name (not `-v0.9`/`-v0.10`) because it is the durable migration entrypoint, not a one-shot.
- **Transform engine: `jq`** with surgical `del`/object-reconstruction, `--indent 2`, **preserving key order** (no `-S`). Records are data files read by value, not byte-compared, so re-serialisation is safe; the render (AC-05) reads parsed values.
- **Idempotent by construction:** every transform is a fixpoint (`quick`→`quick`, absent key stays absent, whitelist projection of an already-pure board is itself).
- Per record type, the script applies:

  **`spec.json`** (strict spec-v1, `additionalProperties:false` top-level and on AC items — allowed AC item keys are exactly `{id, text, ears_pattern, test_refs}`):
  - `del(.schema_version)` (retired; `$schema` carries the version)
  - `.acceptance_criteria |= map(del(.type, .ears_keyword))` — **the shim strips these on read but no Go caller ever normalises spec-v1 (dead branch, see P-2); on disk they must go or strict validation fails**
  - `.effort_complexity.quadrant`: `chore`→`quick`, `epic`→`beast` (axes untouched)
  - ensure `in_scope`/`out_of_scope` exist (default `[]` when absent — historical backfill; this release's 15 already carry real content)

  **`status.json`** (slice-status-v1, `additionalProperties:true`):
  - `.effort_complexity.quadrant`: `chore`→`quick`, `epic`→`beast` only. **`schema_version` is KEPT** (schema tolerates it).

  **`board.json`** (strict board-v1, pure plan) — **whitelist projection**, not the shim's blacklist:
  - reconstruct as `{"$schema", release, tracks: (.tracks|map({id, slices} + optional depends_on))}` — this drops `schema_version`, `release_worktree_path/branch`, **`activity`**, and every track `state`/`worktree_path`/`worktree_branch` in one move, and is future-proof against any other stray derived field.

- The script **skips the invalid-`feature` fix as a code path is unnecessary** (see P-1) but includes a `chore/epic/feature`→canonical map so that *if* a `feature` record ever appears (in `~/projects/fired` or a re-run) it is corrected; for this repo it is a no-op.

### 3b. Fix the invalid `feature` quadrant (AC-02) — see pin P-1

`grep -rn '"quadrant": "feature"' docs/release/` returns **zero matches** on both the track base and `release/v0.1.0`. The record AC-02 targets (reported "near render-drift S03-tui-chrome-rework", which is `epic` high/high) **does not exist in current data** — it was evidently already corrected before this slice, or the original report was inaccurate. Implementation will re-run the grep live, record "0 `feature` records; AC-02 satisfied by absence with grep evidence" in `journal.md`, and the script's map covers the case defensively. **Escalated to the Captain** because AC-02 is phrased as "SHALL be corrected", presupposing existence.

### 3c. Remove S11's normalise shim (AC-04)

- Delete `internal/baton/normalise.go` + `internal/baton/normalise_test.go` + `internal/baton/testdata/normalise/`.
- Remove the two call sites: `internal/state/state.go` `Read()` (the `baton.Normalise("slice-status-v1", …)` block, lines ~523-531) and `internal/board/board.go` `ReadBoard()` (the `baton.Normalise("board-v1", …)` block, lines ~135-138), plus their explanatory comments.
- `state.EffortComplexity.Validate()` **already** accepts only `quick/grind/puzzle/beast` (via `Quadrant()` — it never accepted `chore/epic`; the shim mapped before Validate). So AC-04's "tighten to quick-only" = **the shim removal itself**; the `Validate()` body is confirmed strict and stays unchanged. `effort_complexity_test.go` (declared touchpoint) is updated to drop any shim-dependent legacy-name case and assert the strict enum.
- **Sequencing:** the data migration (3a) runs and commits BEFORE the shim removal so no reader ever meets an un-migrated on-disk record without the tolerance. Both land in this one slice.
- **Fixture blast radius (memory: "S05 strict reader broke board string fixtures"):** ~9 Go test files reference `chore`/`epic` or worktree fields (`internal/run/parallel_test.go`, `internal/router/router_test.go`, `cmd/sworn/{regress,doctor,merge}_test.go`, `internal/state/effort_complexity_test.go`, etc.). Implementation audits each: any that feed a legacy record through the now-removed shim on `state.Read`/`board.ReadBoard` are migrated to canonical values. **The gate is a full `go test -count=1 -timeout 300s ./...`**, which surfaces any missed fixture — run before the state transition, per the standing hazard.

### 3d. Refresh renders (AC-05)

Re-run `sworn render <release>` for each of the **five** releases (positional release, project-root = worktree). Commit the refreshed `index.md`. The fail-closed backstop is `sworn doctor`'s `checkRenderDrift`. S11 already made render derive per-slice state (no dependence on the dropped `tracks[].state`), so pure-plan boards render cleanly — verified during implementation.

### 3e. Validation sweep (AC-03 / AC-06) — see pin P-3

AC-03/AC-06 require proving each record validates against the **vendored v0.10.0** schema via draft-2020-12 `baton.ValidateSchema`. **No existing CLI surface runs `ValidateSchema` over on-disk records** (write paths use the lenient hand-rolled `baton.Validate`; `sworn doctor` checks render-drift + timestamps, not schema conformance). Adding a new CLI command is out of scope ("no Go behaviour change beyond removing the tolerance"). **Proposed:** a records-conformance **Go test** (`internal/state/records_conformance_test.go` or `internal/baton/records_conformance_test.go`) that globs the five releases' `spec.json`/`board.json` and asserts each passes `baton.ValidateSchema("spec-v1"|"board-v1", …)`. This doubles as durable regression (a future un-migrated record fails CI) and satisfies Rule 1 reachability (real records through the real strict validator). Its captured output is the AC-03/AC-06 sweep evidence in `proof.json`. **This adds one test file beyond the declared `touchpoints`** — Captain approval requested (P-3).

## 4. Acceptance-criteria → change traceability

| AC | Satisfied by | Evidence in proof |
|---|---|---|
| AC-01 (zero chore/epic) | 3a quadrant rename (spec+status) | `grep -rn '"quadrant": "chore"\|"epic"' docs/release/` = 0 |
| AC-02 (fix `feature`) | 3b — absent in data; grep-zero + journal note | live grep = 0; escalated P-1 |
| AC-03 (specs valid + in/out_of_scope) | 3a spec transforms + 3e sweep | conformance test output |
| AC-04 (Validate quick-only, shim gone) | 3c | `go test ./internal/state/...` + full suite green |
| AC-05 (renders match, 5 releases) | 3d | `sworn render` ×5 + `sworn doctor` render-drift PASS |
| AC-06 (boards pure-plan valid, 5 releases) | 3a board projection + 3e sweep | board key assertions + conformance test |

## 5. Files to touch

- **New:** `scripts/migrate-records.sh`; one records-conformance test file (P-3).
- **Data (5 release trees):** `docs/release/{2026-06-28-driver-contract, 2026-06-30-sworn-operational-readiness, 2026-07-01-loop-cli-ux, 2026-07-01-release-hygiene, 2026-07-01-render-drift-reconciliation}/**/spec.json` + `status.json` + `board.json` + `index.md`.
- **Code:** delete `internal/baton/normalise.go`, `internal/baton/normalise_test.go`, `internal/baton/testdata/normalise/`; edit `internal/state/state.go` (`Read`), `internal/board/board.go` (`ReadBoard`), `internal/state/effort_complexity_test.go`; audit/fix the ~9 shim-dependent test fixtures.
- **File-ceiling exception** already accepted in the spec (mechanical one-line data edits from one script).

## 6. Design pins (for Captain acknowledgement)

- **P-1 (escalate):** AC-02's invalid `feature` record does not exist in current data (0 matches on branch + integration tip; nearest named record S03-tui-chrome-rework is `epic`). Proposed: satisfy AC-02 by *asserting absence* (grep-zero + journal note) rather than a correction edit. Confirm this satisfies AC-02, or point to the branch/record where `feature` still lives.
- **P-2 (memory-cited / mechanical):** The shim's `spec-v1` case (strip `type`/`ears_keyword`) is **dead code** — no Go caller invokes `Normalise("spec-v1", …)`; specs are only lenient-validated on write. So on-disk specs currently carry `type`/`ears_keyword` that strict v0.10.0 spec-v1 forbids. The migration must strip them (done in 3a) for AC-03 to pass. Flagging so the reviewer knows the migration is *stricter* than the shim on the spec surface by necessity, not scope creep.
- **P-3 (escalate — touchpoint expansion):** AC-03/AC-06 need a strict-schema sweep, but no CLI runs `ValidateSchema` over records and adding one is out of scope. Proposed: a records-conformance **Go test** (one new file beyond declared touchpoints) as both sweep evidence and durable regression. Approve the test-file addition, or direct a jq/structural sweep in the script + proof instead (weaker: not full draft-2020-12).
- **P-4 (mechanical):** board migration is **whitelist-projection**, not the shim's blacklist, to also drop the stray `activity` key (2 boards) that strict board-v1 forbids. Behaviourally sound (pure-plan derived fields), noted so the larger board diff is expected.
- **P-5 (mechanical / memory-cited):** in-flight releases (operational-readiness, release-hygiene) hold diverged record copies on their track branches; this migration lands on the integration line and reaches them via `/implement-slice` Step-0 forward-merge self-heal (2026-06-28 replan-propagation lesson). Named in the spec's R-01; journal will enumerate touched in-flight tracks.

## 7. Risks

- **R-01 (spec):** in-flight track branches read "behind" post-migration — expected, self-healing via forward-merge (see P-5).
- **R-02 (spec):** a missed record leaves the strict reader failing after shim removal — mitigated by AC-01 grep-zero + full `go test ./...` + `sworn doctor`, all run in this slice.
- **R-03 (new):** `jq` re-serialisation enlarges the data diff; acceptable (mechanical migration), and idempotency + the conformance test bound the risk.

## 8. Divergence from plan

None yet. Two necessary extensions beyond the literal spec are surfaced as pins, not silent deviations: the whitelist board projection (P-4, to catch `activity`) and the AC-02 absence-satisfaction (P-1). Both await Captain acknowledgement before code.
