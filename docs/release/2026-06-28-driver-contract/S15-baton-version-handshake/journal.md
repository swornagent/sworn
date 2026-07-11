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
