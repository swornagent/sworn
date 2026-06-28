# Design TL;DR — S17-journeys-declare

## Approach

Declare three Rule-10 critical journeys by:
1. Adding a `no_mock_boundary` field to the `Journey` struct and journeys-v1 schema
2. Writing `.sworn/journeys.json` directly as the ratified artefact
3. Adding a `TestCheck_S17Journeys` unit test that exercises `Check()` with a fixture containing the three journeys

No new Go commands needed — the existing `journey.Check()` and `journey.SaveArtefact()` pipeline is sufficient.

## Key design choices

**`no_mock_boundary` field placement**: Added as an optional `string` field (`json:"no_mock_boundary,omitempty"`) on the `Journey` struct in `internal/journey/journey.go`. The `validateJourneys` structural validator does not walk individual journey items, so no validator code changes are required. The `journeys-v1.json` JSON Schema is updated to document the property (but since there's no `additionalProperties: false` guard, existing artefacts stay valid without it). Type-2 choice — additive, easy to extend.

**Creating `.sworn/journeys.json`**: Write directly as JSON using the known `journeys-v1` shape. Ratified at implementation time with `brad@sawyer.net.au` / `2026-06-28`. This is the fastest path to AC1–AC5 and avoids adding a code-generation sub-command. The file is committed to the track branch and flows to the integration branch via `/merge-track` + `/merge-release`. The spec note "commits to the integration branch" means its ultimate home is `main` — not that we bypass the track flow.

**Test**: `TestCheck_S17Journeys` in `internal/journey/journey_test.go` builds a temp-dir fixture with the three specific journeys (J1 keyless-full-loop, J2 loop-verifier-negative, J3 ship-a-release), ratifies it, saves it, and asserts `Check()` returns `CheckPass`. Uses the same `t.TempDir()` pattern as all other tests in the file — no filesystem dependency on the project root.

## Files to touch

| File | Change |
|---|---|
| `internal/journey/journey.go` | Add `NoMockBoundary string \`json:"no_mock_boundary,omitempty"\`` to `Journey` struct |
| `internal/baton/schemas/journeys-v1.json` | Add `no_mock_boundary` as optional string property inside journey items |
| `internal/journey/journey_test.go` | Add `TestCheck_S17Journeys` covering 3-journey fixture → CheckPass |
| `.sworn/journeys.json` | New file: ratified artefact with J1, J2, J3 |

## Traceability

| AC | Planned change |
|---|---|
| AC1 `.sworn/journeys.json` exists | create `.sworn/journeys.json` |
| AC2 parses as valid journeys-v1 | `SaveArtefact` validates against schema before writing |
| AC3 exactly 3 journeys with correct IDs | JSON file content |
| AC4 each has `no_mock_boundary` | new struct field + JSON content |
| AC5 `artefact.IsRatified` true | `ratification.is_ratified: true` in JSON |
| AC6 `journey.Check()` returns CheckPass | covered by `TestCheck_S17Journeys` |
| AC7 `sworn merge-release` doesn't BLOCK | `sworn journeys --check` exits 0 when file is present + ratified |

## Risks / pins

- **Risk**: `Journey.NoMockBoundary` is a string field — if the schema ever adds an enum, existing artefacts with arbitrary strings would fail. **Mitigation**: schema currently has no enum for this field; noted as forward-compatible.
- **Risk**: The spec's note "commits to the integration branch" could be misread as "bypass the track merge flow". **Decision**: commit to track branch as normal; it reaches `main` via `/merge-track` + `/merge-release`. The spec note is describing the eventual home, not the commit path.
