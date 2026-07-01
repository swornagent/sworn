---
title: 'S15 — spec.json and proof.json records; fix proof bundle sections'
description: 'Emit spec-v1 spec.json from spec.md; emit proof-v1 proof.json from live state; fix generateProof() to derive all sections from live ACs and git state (not constant boilerplate); fix files_changed to use git diff --name-only <start_commit>.'
---

# Slice: `S15-spec-proof-records`

## User outcome

After `sworn run` implements a slice: `spec.json` exists alongside `spec.md` with the acceptance criteria as a machine-readable array; `proof.json` exists with sections derived from live repo state (files_changed from `git diff --name-only <start_commit>..HEAD`, delivered from AC list, not_delivered from open_deferrals, divergence from plan drift); the RTM trace gate reads from `spec.json` `covers_needs` (no more needs:0 on real releases).

## Entry point

`internal/implement/implement.go` `Run()` + `generateProof()` (audit refs: `internal/implement/implement.go:40` for spec.json missing; `:177-191` for proof.md constant sections).

## In scope

- New `internal/implement/spec_record.go`: `WriteSpecRecord(specPath, sliceDir string, st *state.Status) error` — parses spec.md to extract user outcome, in-scope list, AC checks; writes `spec.json` (spec-v1) to `<sliceDir>/spec.json`; spec-v1 shape: `{schema_version, slice_id, release, user_outcome, acceptance_criteria: [{id, text, type, ears_keyword}], covers_needs: [...]}`
- New `internal/implement/proof_record.go`: `WriteProofRecord(specPath, proofPath, sliceDir string, st *state.Status) error` — writes `proof.json` (proof-v1); proof-v1 shape: `{schema_version, slice_id, release, scope, files_changed: [...], test_results: [{command, exit_code, output}], reachability_artifacts: [...], delivered: [...], not_delivered: [...], divergence: [...]}`
- Fix `generateProof()`:
  - `files_changed`: use `git diff --name-only <start_commit>..HEAD` (not `git status --porcelain`); `start_commit` from `st.StartCommit`
  - `delivered`: parse acceptance criteria from spec.md (via `parseAcceptanceCriteria()`); mark each checked item (`- [x]`) as delivered with the test name evidence
  - `not_delivered`: derive from `st.OpenDeferrals` (not hardcoded "None")
  - `divergence`: compare `st.PlannedFiles` to actual `git diff --name-only` output; report unexpected files as divergence
  - Remove the constant "scripts/release-verify.sh" line (the script does not exist per audit)
- RTM trace gate: `internal/gate/trace.go` — add a path that reads `covers_needs` from `spec.json` when it exists (falls back to spec.md markdown parsing when spec.json absent); N-04 resolved: `covers_needs` array in spec.json is always correct for machine-generated specs
- Schema validation: add `validator.Validate("spec-v1", specData)` and `validator.Validate("proof-v1", proofData)` before writing (uses S13 embedded schemas)
- Add `internal/baton/schemas/spec-v1.json` and `internal/baton/schemas/proof-v1.json` to embedded schemas

## Out of scope

- Journeys and attestations records (S16)
- board.json (S14)
- The first-pass gate changes (S12, T3)
- Changing the acceptance criteria from spec.md format (the AC parser reads the existing `- [ ]` checkbox format)

## Planned touchpoints

- `internal/implement/implement.go` (call WriteSpecRecord + WriteProofRecord + fix generateProof())
- `internal/implement/spec_record.go` (new)
- `internal/implement/proof_record.go` (new)
- `internal/gate/trace.go` (add spec.json covers_needs read path)
- `internal/baton/schemas/spec-v1.json` (new embedded schema)
- `internal/baton/schemas/proof-v1.json` (new embedded schema)

## Acceptance checks

- [ ] WHEN `Run()` completes and transitions to `implemented`, THE SYSTEM SHALL write `spec.json` to the slice directory
- [ ] spec.json `covers_needs` array MUST contain at least one element for any slice specced with intake N-NN references (tested against this release's own slices)
- [ ] WHEN `Run()` completes, THE SYSTEM SHALL write `proof.json` with `files_changed` derived from `git diff --name-only <start_commit>..HEAD` (not `git status --porcelain`)
- [ ] `proof.json` `not_delivered` reflects `st.OpenDeferrals` (empty array when no deferrals, not hardcoded "None")
- [ ] `proof.json` `delivered` contains one entry per checked acceptance criterion (`- [x]`) found in spec.md
- [ ] WHEN `internal/gate/trace.go` parses a slice with an existing `spec.json`, it reads `covers_needs` from spec.json and reports `needs > 0` for this release's own intake (N-04 resolved)
- [ ] `grep -rn "scripts/release-verify.sh"` in implement.go returns zero matches after this slice
- [ ] `proof_record_test.go`: test that `files_changed` uses the start_commit diff path, not the working-tree path

## Required tests

- **Unit**: `internal/implement/spec_record_test.go` (new) — parse a sample spec.md and verify covers_needs + AC array extracted correctly
- **Unit**: `internal/implement/proof_record_test.go` (new) — mock git, verify files_changed uses start_commit; verify not_delivered derives from open_deferrals
- **Integration**: `internal/implement/implement_test.go` — assert spec.json and proof.json written after Run()
- **Reachability artefact**: `go test ./internal/implement/... ./internal/gate/... -v` exits 0

## Risks

- Parsing acceptance criteria from spec.md `- [ ]` / `- [x]` patterns requires a reliable markdown list parser; use a simple regex (`^- \[([ x])\]`) rather than a full markdown parser
- `st.StartCommit` may be empty for slices that haven't set it yet; fallback to `git log --oneline -1 HEAD~1` SHA as the base

## Deferrals allowed?

No.
