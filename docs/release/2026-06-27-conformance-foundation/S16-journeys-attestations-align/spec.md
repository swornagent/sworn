---
title: 'S16 — Align journeys-v1 and attestations-v1 to canonical nested shapes'
description: 'Align the internal journeys-v1 and attestations-v1 record shapes to the canonical nested ratification/boundary structure; add $schema field; add validate-on-write for both writers.'
---

# Slice: `S16-journeys-attestations-align`

## User outcome

`sworn journey --generate` and attestation writes produce journeys.json and attestations.json that match the canonical journeys-v1 and attestations-v1 schemas (nested `ratification: {by, at, is_ratified}` not flat fields; `boundary: {name, mock_banned, entitlement_boundary}` not flat); both writers validate against embedded schemas before writing.

## Entry point

`internal/journey/journey.go` `JourneyArtefact` struct (audit ref: `internal/journey/journey.go:76-98`) and `internal/journey/walkthrough.go` attestations writer (audit ref: `walkthrough.go:31-55`).

## In scope

- `internal/journey/journey.go`: update `JourneyArtefact` struct to use nested `Ratification` sub-struct (`{ By, At, IsRatified bool }`) matching canonical shape; update `Ratify()`, `Deratify()`, `NewJourneyArtefact()` to populate the nested struct; add `Schema string \`json:"$schema"\`` field populated with `"https://baton.sawy3r.net/schemas/journeys-v1.json"`
- `internal/journey/journey.go`: update `Journey` struct and `JourneyStep` to align any field names that diverge from canonical journeys-v1 (e.g. `boundary` sub-struct if currently flat fields)
- `internal/journey/walkthrough.go`: update attestations write path to use the canonical nested `attestation: { ratification: {...}, boundary: {...} }` shape matching attestations-v1; add `$schema` field
- Add validate-on-write: in the journey and attestation writers, call `validator.Validate("journeys-v1", data)` and `validator.Validate("attestations-v1", data)` before writing (uses S13 embedded schemas)
- Add `internal/baton/schemas/journeys-v1.json` and `internal/baton/schemas/attestations-v1.json` to embedded schemas
- Update existing journey tests to use the new struct fields

## Out of scope

- Declaring the actual journeys content (S17)
- The no-mock boundary detection keywords (S11)
- Any changes to impact.go, regression.go, shipgate.go

## Planned touchpoints

- `internal/journey/journey.go` (update JourneyArtefact + nested struct + $schema)
- `internal/journey/walkthrough.go` (update attestation shape)
- `internal/baton/schemas/journeys-v1.json` (new embedded schema)
- `internal/baton/schemas/attestations-v1.json` (new embedded schema)

## Acceptance checks

- [ ] `JourneyArtefact` struct in `journey.go` has a nested `Ratification` field (not flat `IsRatified bool`, `RatifiedAt string`, `RatifiedBy string` at the top level); `grep -n "IsRatified bool" internal/journey/journey.go` returns zero results (moved to nested struct)
- [ ] WHEN `JourneyArtefact.Ratify("brad@sawyer.net.au")` is called, the written JSON includes `"ratification": {"by": "brad@sawyer.net.au", "at": "...", "is_ratified": true}` (nested)
- [ ] WHEN a journey is written to disk, the JSON includes `"$schema": "https://baton.sawy3r.net/schemas/journeys-v1.json"`
- [ ] WHEN an attestation is written to disk, the JSON includes `"$schema": "https://baton.sawy3r.net/schemas/attestations-v1.json"` and the nested `ratification` shape
- [ ] WHEN validate-on-write fails (e.g. missing required field), the writer returns an error and does NOT write the file
- [ ] `journey_test.go`: all existing journey tests pass after struct rename; add a round-trip test that writes a ratified artefact and verifies the nested shape

## Required tests

- **Unit**: `internal/journey/journey_test.go` (update existing + add nested-shape round-trip)
- **Reachability artefact**: `go test ./internal/journey/... -v` exits 0

## Risks

- Existing `.sworn/journeys.json` files (if any in the repo) will fail to parse with the new schema; migration needed. Since no `.sworn/journeys.json` currently exists in the repo, this is not a concern.
- The canonical journeys-v1 and attestations-v1 schemas must be sourced from `$HOME/.claude/baton/schemas/` — the implementer must verify this path exists and contains the schemas

## Deferrals allowed?

No.
