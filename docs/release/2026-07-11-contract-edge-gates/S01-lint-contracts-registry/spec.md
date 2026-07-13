---
title: 'S01-lint-contracts-registry'
description: '`sworn lint contracts <release>` grades a releases contracts.json registry, fail-closed: it validates every record against the vendored contracts-v1 schema, flags any spec.json in'
---

# Slice: `S01-lint-contracts-registry`

## User outcome

`sworn lint contracts <release>` grades a release's contracts.json registry, fail-closed: it validates every record against the vendored contracts-v1 schema, flags any spec.json in-scope/AC that names a wire-level artefact (header, endpoint path, env var, schema-version, storage key) with no registry entry, verifies each contract's live_test resolves to a real test, and rejects a surface with two owners or an owner slice whose touchpoints cannot plausibly contain the surface. Absent contracts.json is an advisory warning (exit 0) during the skew window. It follows the existing `sworn lint` subcommand conventions (ac/trace/deps/coverage/design/mock).

## In scope

- A `contracts` subcommand on the existing `sworn lint` surface (cmd/sworn/lint.go dispatch + internal/lint/contracts.go), following the sibling-subcommand pattern
- Validate every contracts.json record against the vendored contracts-v1 schema (baton.ValidateSchema) — FAIL on any non-validating record
- Wire-reference completeness: scan each spec.json in-scope + acceptance_criteria text for wire-level artefacts (header names, endpoint paths, env vars [A-Z_]{4,}, schemaVersion tokens, storage keys) and FAIL on any that has no matching contracts.json entry (heuristic set from the proposal, tuned to avoid false positives on prose)
- live_test resolution: FAIL if a contract's live_test does not resolve to a real test (reuse the trace gate's test_refs resolution logic)
- Ownership sanity: FAIL if two contracts name the same surface with different owners, or if an owner slice's touchpoints cannot plausibly contain the surface (e.g. an http-endpoint owned by a slice with no server-side touchpoint)
- Advisory window: absent docs/release/<release>/contracts.json → single WARN, exit 0 (flips to required when this gate ships — coordinated with baton per the skew policy)

## Out of scope

- Mock-parity checks (fixtures freshness + consumer-mock-import) — S02 owns those
- sworn assemble / assembly-proof (T2)
- Emitting or authoring contracts.json (planner-emitted, baton role-prompt side)
- Changing the vendored contracts-v1 schema (it is fixed upstream; grade against it, never fork under the same $id)

## Acceptance criteria

- [ ] AC-01: When `sworn lint contracts <release>` runs against a release whose contracts.json contains a record that does not validate against the vendored contracts-v1 schema, it SHALL exit non-zero naming the record id and the validation failure.
- [ ] AC-02: When a spec.json in-scope item or acceptance criterion references a wire-level artefact (header name, endpoint path, env var matching [A-Z_]{4,}, schemaVersion, or storage key) that has no matching contracts.json entry, `sworn lint contracts` SHALL exit non-zero naming the slice, the unregistered surface, and the artefact class — proven against the fired 2026-07-10 corpus (S01/S02/S14 with no CP-PUT registry entry → FAIL, seam 1).
- [ ] AC-03: If a contract's live_test does not resolve to a real test (same resolution as the trace gate's test_refs), OR two contracts name one surface with different owners, OR an owner slice's touchpoints cannot plausibly contain the surface, `sworn lint contracts` SHALL exit non-zero naming the specific violation.
- [ ] AC-04: While a release has no contracts.json file, `sworn lint contracts` SHALL emit a single advisory WARN and exit 0 (the skew-window behaviour), never a hard failure.
- [ ] AC-05: `sworn lint contracts` SHALL exit 0 with no findings on a well-formed registry (the baton repo's release-mode-template/contracts.json exemplar), and `go build ./...` + `go test ./internal/lint/... ./cmd/sworn/...` SHALL pass.
