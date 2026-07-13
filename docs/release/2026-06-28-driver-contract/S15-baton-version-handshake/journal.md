# Journal — S15-baton-version-handshake

## 2026-07-11 — Implementer session

**State transitions:** design_review (Coach ack COMMITTED, captain-proceed.md
verdict PROCEED) -> in_progress -> implemented.

**Design review ack verified.** Read `review.md` (Captain verdict PROCEED,
2 pins — 1 mechanical, 1 memory-cited, 0 escalate) and `captain-proceed.md`
(Coach Brad, both pin dispositions ACCEPTED). Applied pin 1 (transcribe
D1-D3 from design.md into `status.json.design_decisions`, S13's
`{id, choice, stake_class, rationale}` shape, no `human_decision` — all
Type-2 noted defaults) as the first act of the in_progress transition,
per the accepted disposition.

**Ground truth re-confirmed against the live S11-landed tree** before
writing code (matches design.md's citations exactly):
- `internal/baton/schemas/embed.go` `SchemaMap` — 9 entries, each `$id`
  grep-confirmed to follow `https://baton.sawy3r.net/schemas/<name>.json`.
- `internal/baton/validator.go` `Validate()`'s switch — exactly 6 cases
  (slice-status-v1, board-v1, spec-v1, proof-v1, journeys-v1,
  attestations-v1).
- `internal/verify/verify.go:283` — the sole production
  `ValidateSchema("verifier-verdict-v1", ...)` call site (grepped
  repo-wide, confirmed no other call sites).
- Graded = 7, Advisory = {contracts-v1, assembly-proof-v1} = 2,
  7+2 = 9 = `len(SchemaMap)`.

**Implementation (per design.md's approach, no deviation):**
- `internal/baton/manifest.go` (new) — `GradeStatus` (`Graded`/`Advisory`),
  `SchemaManifestEntry`, the one hand-authored `schemaGradeStatus` 9-entry
  table (each line cites its call-site source), `SchemaManifest()` (name/
  $id/version always parsed from embedded bytes, never hand-listed),
  `SchemaSkew()` (symmetric-difference check between the classification
  table and the live vendored set).
- `internal/baton/manifest_stub.go` (new) — `schemaMapFn` injectable +
  `SetSchemaMapForTest`/`ClearSchemaMapForTest`, mirroring the existing
  `version_stub.go` pattern exactly.
- `internal/baton/manifest_test.go` (new) — leaf-level unit coverage
  (Rule 1: "fine in addition", not the sole proof of life).
- `cmd/sworn/doctor.go` — `checkSchemaManifest()` + wired into `cmdDoctor`
  as "Group 1b: Baton schema manifest", inserted directly after Group 1
  (matches P1's reviewer-confirmed open slot, no naming collision).
- `cmd/sworn/doctor_test.go` — two reachability tests driven through
  `cmdDoctor` (not the leaf function), per Rule 1:
  `TestDoctorSchemaManifestRendersGradedAndAdvisory` (AC-01) and
  `TestDoctorSchemaSkewFiresOnFixture` (AC-02, injects a `made-up-v1`
  fixture via `SetSchemaMapForTest`, asserts `[WARN]` not `[OK]`).

**Design decisions D1-D3** (all Type-2, noted defaults, transcribed into
`status.json.design_decisions` per the Coach's accepted pin — see that
file for the full text): (D1) manifest fields are 100% mechanically
derived from `SchemaMap`, only the 9-entry graded/advisory table is
hand-authored and skew-checked; (D2) skew renders `[WARN]`, not
`[ERROR]` — does not flip `cmdDoctor`'s exit code; (D3) rejected
scraping `internal/adopt/baton/VERSION`'s free-text `schemas-added:` line
as a second, more fragile parser (the exact pattern ADR-0011 already
deleted elsewhere) in favour of the explicit table + `SchemaSkew()`.

**Reachability proof (Rule 1):** ran `go run ./cmd/sworn doctor` live
against this repo. Group 1b renders all 9 schemas with `$id`/`version`/
status; the 7 graded schemas show `status=GRADED`, `contracts-v1` and
`assembly-proof-v1` show `status=ADVISORY`; `baton/schema-skew` shows
`[OK] declared graded/advisory set matches the vendored schema set`.
Confirmed no regression to existing doctor groups: diffed the full
`[ERROR]` line set of `sworn doctor`'s output before vs. after this
slice's changes (via `git stash` of just the S15 files) — byte-identical
75 pre-existing `[ERROR]` lines both times (unrelated repo-state
staleness: future-dated `status.json` timestamps and stale
`index.md`/`board.json` render drift in other, unrelated in-flight
releases — not introduced by S15, and not something S15's scope covers).

**No deviation from design.md.** Every file, function, and test named in
the design's "Files to touch" / "AC -> change traceability" sections
landed as planned.

**Full-suite backstop:** `go test -count=1 -timeout 300s ./...` — 47
packages `ok`, 0 `FAIL` (one `[no test files]` package, `internal/verdict`,
pre-existing and unrelated). Newline-eating-edit corruption grep run
against all 5 touched/new files — no match (sworn#96 hazard). `gofmt -l`
and `go vet` clean on all touched files.

**Not delivered (explicitly out of scope, per spec.json):** grading
contracts-v1/assembly-proof-v1 (follow-on contract-edge-gates release);
changing the vendored pin or schema files (S11's scope); the record data
migration (S12's scope); network auto-sync (doctor declares vendored/
graded state, does not phone home).

## Verifier verdicts received

### 2026-07-11 — Fresh-context verifier (Rule 7) — PASS

Verified inside track worktree `track/2026-06-28-driver-contract/T7-baton-revendor`
(drift vs `release-wt/2026-06-28-driver-contract` = 0, no forward-merge needed).
`start_commit` = `c393459d769c8f98c79c0d8b2fc70aa87857a0bb`; scope re-derived
from `git diff --name-only <start_commit>` (no merge noise) — matches
`proof.json.files_changed` exactly.

Re-ran independently (not trusted from proof.json):
- `go build ./...` — clean.
- `go vet ./internal/baton/... ./cmd/sworn/...` — clean.
- `gofmt -l` on all 5 touched/new `.go` files — clean.
- Newline-eating-edit hazard grep (sworn#96 pattern) on touched files — no hits.
- `go test ./cmd/sworn/... ./internal/baton/...` (AC-03's named command) — PASS.
- `go test -count=1 -timeout 300s ./...` (full suite, fresh cache) — 47 packages
  `ok`, 0 FAIL, 1 no-test-files (`internal/verdict`, pre-existing/unrelated).
- `TestDoctorSchemaManifestRendersGradedAndAdvisory` and
  `TestDoctorSchemaSkewFiresOnFixture` individually — PASS; confirmed both
  drive `cmdDoctor` (the CLI entry point), not the leaf functions in isolation
  (Rule 1) — leaf coverage in `internal/baton/manifest_test.go` is additional,
  not sole proof of life.

Code-read confirmations:
- `SchemaManifest()`/`SchemaSkew()` (`internal/baton/manifest.go`) source the
  schema set from `schemaMapFn()` = `schemas.SchemaMap` (the go:embed vendored
  ground truth, `internal/baton/schemas/embed.go`) — not a hand-typed literal.
  Only the 9-entry `schemaGradeStatus` graded/advisory classification table is
  hand-authored, and `SchemaSkew()` cross-checks its key set against the live
  vendored set on every doctor run (D1).
- `contracts-v1` and `assembly-proof-v1` are present in `schemas.SchemaMap` and
  classified `Advisory`; both are absent from `Validate()`'s grader switch
  (`internal/baton/validator.go`, 6 cases: slice-status-v1, board-v1, spec-v1,
  proof-v1, journeys-v1, attestations-v1) and have no production
  `ValidateSchema(...)` call site (grepped repo-wide) — correctly ADVISORY, not
  silently over- or under-classified.
- `verifier-verdict-v1` is `Graded` via the genuine production call site
  `internal/verify/verify.go:283` (`ValidateSchema("verifier-verdict-v1", ...)`,
  ADR-0011 keystone path) — not fabricated.
- Group 1b (`checkSchemaManifest`) is wired into `cmdDoctor` between Group 1
  and Group 2; skew renders `[WARN]` and does not set `hasError` (D2, confirmed
  by reading the `if skew := ...` branch — only the `else` WARN path, no
  `hasError = true`).

Own reachability artefact (not just proof.json's claim): ran
`go run ./cmd/sworn doctor` live in the track worktree — Group 1b renders all
9 vendored schemas with correct `$id`/`version`/`GRADED`|`ADVISORY` status,
`baton/schema-skew` shows `[OK]`. Independently reproduced the A/B
no-regression check: created a detached worktree at `start_commit`, diffed the
full sorted `[ERROR]` line set of `sworn doctor`'s stdout against the same
worktree at `HEAD` — byte-identical 75 lines both times (0 added, 0 removed),
all attributable to pre-existing unrelated repo staleness in other releases,
not S15. Skew-fixture behaviour independently reproduced too:
`TestDoctorSchemaSkewFiresOnFixture` and the two `manifest_test.go` skew tests
(`TestSchemaSkewFiresOnExtraUnclassifiedSchema`,
`TestSchemaSkewFiresOnMissingVendoredSchema`) all fire `[WARN]`, never `[OK]`,
on injected fixtures.

Gates walked: (1) user-reachable outcome exists — PASS; (2) planned
touchpoints match actual changed files — PASS (all changed non-doc files
within `cmd/sworn/doctor.go`, `cmd/sworn/doctor_test.go`, `internal/baton/`;
`internal/adopt/baton/VERSION` untouched, allowed-but-not-required, D3
explains why); (3) required tests exist and exercise the integration point —
PASS; (4) reachability artefact proves the user path — PASS (own live run +
own A/B diff, see above); (5) no silent deferrals or placeholder logic — PASS
(grep clean for TODO/FIXME/deferred/placeholder on changed files); (6) claimed
scope matches implemented scope — PASS (all 4 `delivered[]` items in
proof.json independently verified against live code + passing tests;
`not_delivered`/`divergence` both empty and no undisclosed gap found).

**Verdict: PASS.** No violations. Next step: `T7-baton-revendor`'s remaining
slice `S12-record-migration` is in `design_review`, not yet `implemented` —
not next for `/verify-slice`. The track is not yet complete
(`S12-record-migration` still open).
