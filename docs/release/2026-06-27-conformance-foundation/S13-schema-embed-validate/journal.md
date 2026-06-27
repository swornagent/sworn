# S13-schema-embed-validate — Journal

## 2026-06-28T08:40:00Z — Start implementation

- State: planned → in_progress
- Approach: structural required-fields check (per spec Risks section — option b, no new dep)
- Schema vendored from canonical baton location
- Go stdlib `encoding/json` for structure check; no third-party JSON schema library

## 2026-06-28T09:00:00Z — Implementation complete

- State: in_progress → implemented
- Created `internal/baton/schemas/embed.go` with `//go:embed` directive
- Vendored canonical `slice-status-v1.json` from `/home/brad/projects/baton/schemas/`
- Created `internal/baton/validator.go` with `Validate()` function
  - Structural required-fields check per spec Risks option (b)
  - Required: slice_id, release, track, state (non-empty strings)
  - Also validates: state enum, $schema URI, verification.result presence
- Modified `internal/state/state.go` `Write()`:
  - Sets `$schema` to canonical URI before marshalling
  - Validates marshalled JSON before `os.WriteFile`
  - Fails closed on invalid data
- Added `internal/baton/validator_test.go`: 13 test cases covering valid/invalid/empty/edge
- Extended `internal/state/state_test.go`:
  - `TestWrite_MalformedStatus`: verifies fail-closed on empty slice_id
  - `TestWrite_SetsCanonicalSchema`: verifies $schema replacement and no example.com
  - Updated 6 existing test fixtures to include required fields (release, track, verification)
- All 13 validator tests + 19 state tests pass (0 failures)
- AC4 note: grep for `"example.com"` finds the negative assertion in the new test
  (intentional — it tests that example.com does NOT appear in output). The old
  `"https://example.com/..."` Schema value in `TestReadWrite_RoundTrip` was replaced.
- First-pass script: dark-code false positives on "deferred" state enum value (legitimate),
  `PLAYWRIGHT_OPTIN` unbound variable (script bug, not slice issue)
- Deferral: Full JSON Schema library validation (ADR-0007)