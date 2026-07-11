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
