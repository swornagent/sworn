# Design TL;DR — S15-baton-version-handshake

**Release:** 2026-06-28-driver-contract · **Track:** T7-baton-revendor · **Slice order:** S11 → S15 → S12
**Depends on:** S11-baton-revendor (verified, landed in this same track worktree) — the vendored v0.10.0 schema set (`internal/baton/schemas/*.json` + `embed.go` `SchemaMap`) and `internal/adopt/baton/VERSION` pin this slice reads.

## User outcome (from spec.json)

`sworn doctor` declares which Baton schema versions this binary grades — an explicit graded-schema-version manifest (spec-v1, board-v1, slice-status-v1, proof-v1, journeys-v1, attestations-v1, verifier-verdict-v1, and the newly-vendored contracts-v1 / assembly-proof-v1 with their graded-or-advisory status) — so a future protocol/runner skew is a VISIBLE warning at doctor time, not a silent behaviour gap.

## Ground truth confirmed in the S11-landed tree

- `internal/baton/schemas/embed.go` `SchemaMap` has exactly 9 entries: `slice-status-v1`, `board-v1`, `spec-v1`, `proof-v1`, `journeys-v1`, `attestations-v1`, `verifier-verdict-v1`, `contracts-v1`, `assembly-proof-v1`. This is the vendored ground truth — built from `//go:embed` directives over the actual JSON files, compile-time verified to exist.
- Every schema's `$id` follows `https://baton.sawy3r.net/schemas/<name>.json` where `<name>` already carries the version (`spec-v1` → version `v1`) — confirmed by grepping `"$id"` across all 9 files.
- `internal/baton/validator.go` `Validate()`'s switch dispatches exactly 7 names: `slice-status-v1`, `board-v1`, `spec-v1`, `proof-v1`, `journeys-v1`, `attestations-v1` (6 cases) — plus `verifier-verdict-v1`, which is graded exclusively via the newer `baton.ValidateSchema()` draft-2020-12 path (`internal/verify/verify.go:283`, the ADR-0011 keystone path). Neither `Validate()`'s switch nor any `ValidateSchema(...)` call site names `contracts-v1` or `assembly-proof-v1` — confirmed by grep. So the **graded set = 7**, the **advisory set = 2**, and 7+2 = 9 = `len(SchemaMap)`. This is the manifest's classification, traced to real call sites, not asserted.
- `internal/adopt/baton/VERSION`'s `schemas-added:` line already says `contracts-v1, assembly-proof-v1 (v0.10.0, vendored-advisory — ... graders are the follow-on contract-edge-gates release)` — free-text confirmation of the same fact, not a machine source (see Design-level pins, P2).

## Approach (per acceptance criterion)

### AC-01 — graded-schema-version manifest rendered by `sworn doctor`

New file `internal/baton/manifest.go`:

- `type GradeStatus string` with `Graded = "GRADED"` / `Advisory = "ADVISORY"`.
- `type SchemaManifestEntry struct { Name, ID, Version string; Status GradeStatus }`.
- `schemaGradeStatus map[string]GradeStatus` — the **one** hand-authored table (7 `Graded` + 2 `Advisory`), each entry commented with the call site that makes it true (`Validate() switch` / `verify.go ValidateSchema`) so it stays auditable, not asserted.
- `SchemaManifest() ([]SchemaManifestEntry, error)` — iterates `schemas.SchemaMap` (sorted by name for determinism), and for each entry: parses `$id` straight out of the embedded JSON bytes (`json.Unmarshal` into a `struct{ ID string \`json:"$id"\` }` — never hand-typed), derives `Version` from the name's trailing `-vN` via one small regexp, and looks up `Status` from `schemaGradeStatus` (empty string if unclassified — see AC-02). **Nothing in this function hand-lists a schema name** (R-01): the only literal names in the file are the 9 keys of `schemaGradeStatus`, which the skew check (AC-02) keeps honest against `SchemaMap`.
- `cmd/sworn/doctor.go`: new `checkSchemaManifest() []checkResult`, wired into `cmdDoctor` as **"Group 1b: Baton schema manifest"** (printed unconditionally, immediately after Group 1 — mirrors the existing "1 → 2 → 2b" sub-lettering convention). One `checkResult` per schema: `name: "baton/schema-manifest/<schema-name>"`, `detail: "$id=<id> version=<version> status=<GRADED|ADVISORY>"`, `level: levelOK` (or `levelWarn` if `Status == ""`, i.e. unclassified — folds AC-02's per-entry case into the same render path). This guarantees the manifest names every graded schema and both advisory schemas explicitly in the doctor output.

### AC-02 — skew check fires on a deliberately-skewed fixture

- `SchemaSkew() []string` (same file) compares `schemaGradeStatus`'s key set against the live `schemas.SchemaMap` key set (both sorted) and returns one line per disagreement: a vendored schema with no classification, or a classified name no longer vendored. Empty slice = no skew.
- `checkSchemaManifest()` appends one final `checkResult` `"baton/schema-skew"`: `levelOK` (`"declared graded/advisory set matches the vendored schema set"`) when `SchemaSkew()` is empty, else `levelWarn` with the joined skew lines — **never a silent OK** (matches AC-02's literal wording; WARN, not necessarily a doctor-exit-1 ERROR — see D2).
- **Test seam** (R-01's other half — the mitigation, not just the source): `internal/baton/manifest_stub.go`, parallel to the existing `version_stub.go` pattern — `schemaMapFn func() map[string][]byte` is the injectable source (`SchemaManifest`/`SchemaSkew` call it, not `schemas.SchemaMap` directly); `SetSchemaMapForTest(m map[string][]byte)` / `ClearSchemaMapForTest()` let a test swap in a fixture map (e.g. an extra `"made-up-v1"` key absent from `schemaGradeStatus`, or a copy of `SchemaMap` with `"contracts-v1"` deleted) without touching real embedded files. `cmd/sworn/doctor_test.go` drives this **through `cmdDoctor`** (not just the leaf function) so the skew WARN is proven reachable at the CLI affordance, per Rule 1: inject the skew fixture, run `cmdDoctor`, assert stdout shows `[WARN]` (not `[OK]`) for `baton/schema-skew`.

### AC-03 — build, targeted suites, no regression to existing doctor groups

- No existing `checkXxx` function, group ordering, or `checkResult` shape changes — Group 1b is additive only, inserted between Group 1 and Group 2's `fmt.Println()` calls.
- `go build ./...` and `go test ./cmd/sworn/... ./internal/baton/...` are the AC-03-cited commands; the full `go test -count=1 -timeout 300s ./...` backstop (hazard: newline-eating-edit + strict-reader-fixture scars) runs before the `implemented` transition regardless.

## Files to touch (⊆ spec touchpoints)

- `internal/baton/manifest.go` (new) — `GradeStatus`, `SchemaManifestEntry`, `schemaGradeStatus`, `SchemaManifest()`, `SchemaSkew()`, `schemaID()`, `parseSchemaVersion()`.
- `internal/baton/manifest_stub.go` (new) — `schemaMapFn` injectable + `SetSchemaMapForTest`/`ClearSchemaMapForTest`, mirroring `version_stub.go`.
- `internal/baton/manifest_test.go` (new) — leaf-level unit coverage of `SchemaManifest()`/`SchemaSkew()` (fine in addition, per Rule 1; not the sole proof of life).
- `cmd/sworn/doctor.go` — `checkSchemaManifest()` + wiring into `cmdDoctor` as Group 1b.
- `cmd/sworn/doctor_test.go` — reachability tests driven through `cmdDoctor` (see AC → change traceability).
- `internal/adopt/baton/VERSION` — **read-only** touchpoint. Confirmed the vendoring is S11's scope, not this slice's; this slice only reads `baton.Version()` (already exposed) for context, does not write the file. No line changes.

## Decision list

| # | Decision | Type (Rule 9) | Status |
|---|----------|---------------|--------|
| **D1** | Vendored schema **names + `$id` + version** are 100% derived from `schemas.SchemaMap` (go:embed ground truth) — zero hand-typed name list. Only the graded/advisory **classification** is a small (9-entry) hand-authored table, and it is cross-checked against `SchemaMap` by `SchemaSkew()` so it cannot silently drift (R-01's mitigation). | Type-2 (narrow, local, easily revisited) | Proposed default |
| **D2** | Skew is surfaced as `[WARN]`, not `[ERROR]` — it does not flip `cmdDoctor`'s `hasError` / exit code. Matches AC-02's literal wording ("clearly-flagged WARN/non-OK"), and matches the existing convention that Groups 2/3/4 are visibility-only while Group 1's embedded-prompt/pin checks are the ones that gate exit code. A structural failure (malformed embedded schema JSON — should be impossible given `//go:embed`'s compile-time guarantee) still returns `levelError` and gates, fail-closed. | Type-2 (narrow — doctor's own WARN/ERROR convention, already established) | Proposed default |
| **D3** | Rejected an alternative: parsing `internal/adopt/baton/VERSION`'s free-text `schemas-added: ... vendored-advisory` line as the machine source for the advisory set. That is exactly the fragile prose-scraping pattern ADR-0011 already deleted elsewhere in this codebase (the verifier-verdict prose scraper). The small explicit Go table + `SchemaSkew()` against `SchemaMap` is the deliberate choice instead — it satisfies R-01's "where possible" without adding a second, more fragile parser. | Type-2 (implementation-detail; no external contract) | Proposed default |

## Design-level pins for the reviewer

- **P1 (mechanical).** Group ordering/naming: "Group 1b: Baton schema manifest", inserted directly after Group 1 (embedded prompt integrity) and before Group 2 (repo artifact audit). Flagging so the reviewer can veto placement/label before code — cheap to move.
- **P2 (memory-cited).** The graded/advisory classification table (`schemaGradeStatus`) is the one piece of this design that is not mechanically derived — it is a 9-entry table with the true source (`Validate()`'s switch cases / the single `ValidateSchema("verifier-verdict-v1", ...)` call site) cited inline. Fully auto-deriving "graded" by introspecting which schema names `Validate()`'s switch handles at runtime (e.g. probing with sentinel invalid payloads and pattern-matching the "no validation rules for schema" error string) was considered and rejected as needless complexity for a low/low chore slice, and as a second flavor of the same fragile-parsing pattern D3 rejects. `SchemaSkew()` is the compensating control: any future vendoring bump that adds/removes a `SchemaMap` entry without updating this table WARNs, which is the whole point of the slice (memory: `feedback_rules_capture_not_omniscience` — the loop doesn't self-notice cross-cutting drift; a human-owned table + a fail-loud skew check is the intended shape, not a self-noticing heuristic).
- **P3 (mechanical).** Test fixture construction for AC-02 uses `SetSchemaMapForTest` to inject either (a) an extra unclassified name, or (b) `SchemaMap` minus one classified name — either proves `SchemaSkew()` fires. Implementation will pick whichever renders the clearer test name; not a design-level fork.

## AC → change traceability

AC-01 → `internal/baton/manifest.go` (`SchemaManifest`, `schemaID`, `parseSchemaVersion`) + `cmd/sworn/doctor.go` (`checkSchemaManifest`, Group 1b wiring) + `doctor_test.go` reachability test asserting the rendered output names all 9 schemas incl. `contracts-v1`/`assembly-proof-v1` as `ADVISORY` and the other 7 as `GRADED`.
AC-02 → `internal/baton/manifest.go` (`SchemaSkew`) + `manifest_stub.go` (`SetSchemaMapForTest`) + `doctor_test.go` reachability test that injects a skew fixture, runs `cmdDoctor`, and asserts `[WARN]` (not `[OK]`) for `baton/schema-skew`.
AC-03 → no existing check/group signature changes; `go build ./...` + `go test ./cmd/sworn/... ./internal/baton/...` + full-suite backstop before `implemented`.

## Divergence from plan

None. D1–D3 are defaults proposed at design time (Type-2, narrow/reversible per Rule 9) rather than spec-mandated; P1–P3 surface them for the Captain to veto before code, per Rule 9's calibration (this slice has no Type-1 / architecturally-significant choices — it is a declarative manifest read entirely from data S11 already vendored).
