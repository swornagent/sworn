---
title: 'S13 — Embed baton schemas; fail-closed validate-on-write'
description: 'Embed the baton/*-v1.json schemas in the binary; add fail-closed JSON-schema validation to state.Write() and other record writers; replace the example.com $schema placeholder.'
---

# Slice: `S13-schema-embed-validate`

## User outcome

Any code path that writes a Baton record (status.json via `state.Write()`) automatically validates the data against the embedded schema before writing; a malformed or drifted record fails with a descriptive error rather than writing silently. The `$schema` field in written records points to the canonical baton schema URI, not the `example.com` placeholder.

## Entry point

`internal/state/state.go` `Write()` function (audit ref: `internal/state/state.go:184-192`); also the `internal/run/run.go:293` record-write call.

## In scope

- Vendor canonical `baton/schemas/slice-status-v1.json` (copy from `$HOME/.claude/baton/schemas/` or the canonical location) into `internal/baton/schemas/` (new directory, embedded via `//go:embed`)
- New `internal/baton/validator.go`: `Validate(schemaName string, data []byte) error` — loads the embedded schema and validates the JSON using a stdlib-compatible JSON schema checker (see Risks for library note)
- `internal/state/state.go` `Write()`: before `os.WriteFile`, call `validator.Validate("slice-status-v1", data)`; return error if validation fails
- `internal/state/state.go` `Write()`: set the `$schema` field on the Status struct to `"https://baton.sawy3r.net/schemas/slice-status-v1.json"` before marshalling
- Tests: `internal/state/state_test.go` — add a test that writes a deliberately malformed Status (missing required fields) and asserts Write() returns an error

## Out of scope

- Validating board.json, spec.json, proof.json, journeys.json (those are S14, S15, S16 respectively)
- Adding a JSON schema library (see Risks — use a minimal schema validator or structural check only)
- Validating records that are read (read-time validation is a separate concern)

## Planned touchpoints

- `internal/baton/schemas/` (new directory with embedded schema files)
- `internal/baton/validator.go` (new)
- `internal/baton/validator_test.go` (new)
- `internal/state/state.go` (Write() validation + $schema field, DOCUMENTED SHARED: T4 owns Write() §184; T7 owns Dispatch §80)

## Acceptance checks

- [ ] `internal/baton/schemas/slice-status-v1.json` is embedded in the binary (verifiable via `//go:embed` directive and test that reads it from the embedded FS)
- [ ] WHEN `state.Write()` is called with a Status that fails schema validation (e.g. missing required `slice_id`), THE SYSTEM SHALL return an error and NOT write the file
- [ ] WHEN `state.Write()` is called with a valid Status, THE SYSTEM SHALL write the file with `"$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json"` at the top level
- [ ] `grep -rn '"example.com"' internal/state/` returns zero results after this slice
- [ ] `validator_test.go`: tests a valid slice-status-v1 payload (passes), a payload missing required field (fails), an empty object (fails)

## Required tests

- **Unit**: `internal/baton/validator_test.go` — valid/invalid/empty payload scenarios
- **Unit**: `internal/state/state_test.go` — extend Write() test with a malformed-status assertion
- **Reachability artefact**: `go test ./internal/baton/... ./internal/state/... -v` exits 0

## Risks

- JSON schema validation without a third-party library: Go stdlib has no JSON schema support. Options: (a) use a lightweight vendored library (xeipuuv/gojsonschema or similar, requires ADR if new dep), (b) use a structural required-fields check that is sufficient for the slice-status-v1 shape. Prefer option (b) for this slice to avoid a new dep; option (a) is a follow-up ADR. The structural check must at minimum verify required string fields are non-empty (`slice_id`, `release`, `track`, `state`).
- Existing test fixtures may not have `$schema` set — the Write() change will add it automatically, but any tests that do a byte-for-byte JSON comparison will need updating

## Deferrals allowed?

Yes — full JSON-schema validation (option a) may be deferred to a follow-up ADR if the structural check (option b) is insufficient. Rule 2: Why = new dep requires ADR-0007 process; Tracking = ADR-0007 (new ADR for JSON schema library); Acknowledged = Brad, 2026-06-27.
