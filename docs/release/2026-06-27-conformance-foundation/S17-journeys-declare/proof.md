# Proof Bundle — S17-journeys-declare

**Release:** 2026-06-27-conformance-foundation  
**Scope:** Declare three Rule-10 critical journeys in `.sworn/journeys.json` using the canonical journeys-v1 shape; ratify and commit so `journey.Check()` passes.

## Files changed

| File | Change |
|---|---|
| `.sworn/journeys.json` | New: ratified artefact with J1, J2, J3 |
| `internal/journey/journey.go` | Add `NoMockBoundary` field to `Journey` struct |
| `internal/journey/journey_test.go` | Add `TestCheck_S17Journeys` |
| `internal/baton/schemas/journeys-v1.json` | Add `no_mock_boundary` property |
| `docs/release/.../S17-journeys-declare/design.md` | Pin 4: update AC7 traceability |
| `docs/release/.../S17-journeys-declare/status.json` | State: in_progress → implemented |

## Test results

**`go test ./internal/journey/... -v -run TestCheck`** — PASS (4/4)
- TestCheck_MissingArtefact: PASS
- TestCheck_UnratifiedArtefact: PASS
- TestCheck_RatifiedArtefact: PASS
- TestCheck_S17Journeys: PASS (covers AC3, AC4, AC6, Pin 2, Pin 3)

**`go test ./internal/journey/...`** — PASS (53/53)

## Reachability artefact

`go test ./internal/journey/... -v -run TestCheck` exits 0. `TestCheck_S17Journeys` exercises `Check()` against a fixture matching the committed `.sworn/journeys.json`, asserts `CheckPass`, validates the committed artefact against the embedded journeys-v1 schema (Pin 2), and asserts non-empty `NoMockBoundary` on all 3 journeys (Pin 3).

## Delivered

- AC1: `.sworn/journeys.json` exists → `.sworn/journeys.json`
- AC2: parses as valid journeys-v1 → `TestCheck_S17Journeys`: `baton.Validate("journeys-v1", data)` passes (Pin 2)
- AC3: exactly 3 journeys with correct IDs → `TestCheck_S17Journeys`: asserts `len==3` and all three IDs present
- AC4: each journey has `no_mock_boundary` → `TestCheck_S17Journeys`: asserts `NoMockBoundary != ""` on each (Pin 3)
- AC5: `artefact.IsRatified` true → `.sworn/journeys.json`: `ratification.is_ratified: true`, `ratified_by: brad@sawyer.net.au` (Pin 5)
- AC6: `journey.Check()` returns CheckPass → `TestCheck_S17Journeys`: asserts `result == CheckPass`
- AC7: journey gate satisfied (transitive via S05) → AC6 satisfied; S05 wires `Check()` into merge-release gate (Pin 4)
- `NoMockBoundary` field on `Journey` struct → `internal/journey/journey.go`
- `no_mock_boundary` in journeys-v1 schema → `internal/baton/schemas/journeys-v1.json`

## Not delivered

- `reachability_test_path` for each journey is TBD (manual attestation at ship cutover). Why: real-infra walk requires a deployed release. Tracking: `/mark-shipped` ceremony. Acknowledged: Brad, 2026-06-27. (Pre-existing deferral.)

## Divergence from plan

- Spec note: "`.sworn/journeys.json` commits to the integration branch directly" — reinterpreted per design.md: file committed to track branch per track-mode flow; reaches integration branch via `/merge-track` + `/merge-release`. This is the correct flow for all track-mode artefacts.
