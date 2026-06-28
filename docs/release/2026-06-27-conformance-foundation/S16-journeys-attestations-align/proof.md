# Proof Bundle — S16-journeys-attestations-align

## Scope

Align journeys-v1 and attestations-v1 record shapes to canonical nested `ratification`/{`boundary`} structure, add `$schema` fields, and add validate-on-write for both writers.

## Files changed

```
cmd/sworn/journeys.go
docs/release/2026-06-27-conformance-foundation/S16-journeys-attestations-align/status.json
internal/baton/schemas/attestations-v1.json
internal/baton/schemas/embed.go
internal/baton/schemas/journeys-v1.json
internal/baton/validator.go
internal/journey/impact.go
internal/journey/journey.go
internal/journey/journey_test.go
internal/journey/walkthrough.go
internal/journey/walkthrough_test.go
```

## Test results

```
$ go test ./internal/journey/... -v
=== RUN   TestImpactAnalysis_MissingArtefact
--- PASS: TestImpactAnalysis_MissingArtefact (0.00s)
=== RUN   TestImpactAnalysis_UnratifiedArtefact
--- PASS: TestImpactAnalysis_UnratifiedArtefact (0.00s)
=== RUN   TestImpactAnalysis_TouchedJourneys
--- PASS: TestImpactAnalysis_TouchedJourneys (0.00s)
...
=== RUN   TestSaveAttestations_ValidateOnWrite
--- PASS: TestSaveAttestations_ValidateOnWrite (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/journey	0.040s
```

51 tests pass, 0 fail. `go vet ./...` is clean.

## Reachability artefact

`go test ./internal/journey/... -v` exits 0 — all unit + integration tests covering journey and attestation models pass.

## Delivered

- [x] `JourneyArtefact` struct uses nested `Ratification` sub-struct (`{By, At, IsRatified}`), not flat fields. Evidence: `internal/journey/journey.go:75-102`
- [x] `Ratify("brad@sawyer.net.au")` writes nested JSON: `"ratification": {"by": "brad@sawyer.net.au", "at": "...", "is_ratified": true}`. Evidence: `TestRatify_NestedShapeRoundtrip` PASS
- [x] Journey JSON includes `"$schema": "https://baton.sawy3r.net/schemas/journeys-v1.json"`. Evidence: `TestRatify_NestedShapeRoundtrip` validates `$schema` key
- [x] Attestation JSON includes `"$schema": "https://baton.sawy3r.net/schemas/attestations-v1.json"` with nested `ratification` and `boundary`. Evidence: `TestSaveAttestations_NestedShapeRoundtrip` PASS
- [x] Validate-on-write: serializer calls `baton.Validate("journeys-v1", data)` / `baton.Validate("attestations-v1", data)` before writing. Evidence: `internal/journey/journey.go:196` and `internal/journey/walkthrough.go:143`
- [x] Embedded schemas: `internal/baton/schemas/journeys-v1.json` and `internal/baton/schemas/attestations-v1.json` created and embedded via `//go:embed`. Evidence: `internal/baton/schemas/embed.go:41-48`
- [x] All existing tests pass after struct rename. Evidence: 51/51 PASS

## Not delivered

None. All in-scope items delivered.

## Divergence from plan

- **Canonical schemas not found at `~/.claude/baton/schemas/`**: The spec risk noted canonical schemas should be sourced from `$HOME/.claude/baton/schemas/`. These do not exist at that path. Schemas were designed from the spec's description of required nested shapes (`ratification`, `boundary`) and the existing `embed.go` / `validator.go` patterns from S13-S15.
- **Validator allows empty `by`/`at` when unratified**: The journeys-v1 and attestations-v1 validators require `by` and `at` to be non-empty only when `is_ratified` is true. When unratified, the keys must be present but may be empty strings. This allows `SaveArtefact` to work before `Ratify()` is called, which existing tests and the `DraftTemplate` flow rely on.

## First-pass script output

```
FIRST-PASS PASS
checks passed: 23
checks failed: 0
```