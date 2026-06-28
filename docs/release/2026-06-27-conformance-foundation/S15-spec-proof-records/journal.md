# Journal — S15-spec-proof-records

## Session 1: 2026-07-25 — Implementation

**State transition:** planned → in_progress → implemented

### Decisions

1. **spec_record.go and proof_record.go as separate files.** Each handles one record type. `WriteSpecRecord` parses spec.md and writes spec.json. `WriteProofRecord` gathers live repo state and writes proof.json. Both are called from `Run()` after the agent loop completes.

2. **generateProof() refactored rather than replaced.** The existing `generateProof()` function that writes proof.md was refactored to:
   - Use `git diff --name-only <start_commit>..HEAD` for files_changed (not `git status --porcelain`)
   - Derive `delivered` from parsing acceptance criteria in spec.md
   - Derive `not_delivered` from `st.OpenDeferrals`
   - Derive `divergence` from comparing planned_files to actual git diff
   - Remove the constant `scripts/release-verify.sh` reference (AC 7)

3. **parseCoversNeeds in trace.go prefers spec.json.** The RTM trace gate now reads `covers_needs` from `spec.json` when it exists, falling back to the regex-based status.json parsing for older slices.

4. **Schema validation for spec-v1 and proof-v1.** Both new schemas get structural required-fields validation in `baton.Validate()`, following the same pattern as slice-status-v1 and board-v1.

5. **Existing test fixtures fixed for validator.** The S13 validator added a `verification.result` required field check. Several pre-existing test fixtures (in implement_test.go, ready_test.go) and gate/trace_test.go had to be updated to include `"verification": {"result": "pending"}`.

### Trade-offs

- **AC parsing uses regex, not a full markdown parser.** Per spec Risks section, `^- \[([ x])\]` is sufficient for the checkbox format used in spec.md.
- **Proof.json and spec.json are supplementary, not replacements.** proof.md and spec.md remain the human-readable canonical forms. The JSON records enable machine consumers (RTM trace gate, board oracle, TUI).

### Files changed

See `git diff --name-only 2bffa27..HEAD` for the full list (13 files: 2 new schemas, 2 new source files, 2 new test files, 7 modified files).
---

## Verifier verdicts received

### Verdict 1 — PASS (2025-07-25)

**Verifier session:** fresh, artefact-only
**Verified against commit:** 8deee4b4d342e11b0a0cb659c0a5d3ac2f2e572f

**Gate results:**
- Gate 1 (User-reachable outcome): PASS — Run() → WriteSpecRecord/WriteProofRecord wired through internal/run/slice.go → cmd/sworn/
- Gate 2 (Planned touchpoints match): PASS — 7 extra files are structural dependencies (test files, embed.go, validator.go) implied by spec scope
- Gate 3 (Required tests exercise integration): PASS — all tests re-run and pass (130+ tests, 0 failures)
- Gate 3b (AC satisfaction LLM): SKIPPED (no LLM provider configured)
- Gate 4 (Reachability artefact): PASS — go test ./internal/implement/... ./internal/gate/... -v exits 0, verified fresh
- Gate 4b (Semantic coverage LLM): SKIPPED (no LLM provider configured)
- Gate 5 (No silent deferrals): PASS — two hits are false positives (state map value + ADR-0007 scope boundary)
- Gate 6 (Design conformance): PASS — no design-fidelity config (non-UI project)
- Gate 7 (Claimed scope matches implemented): PASS — all 12 Delivered items verified against live code

**Next step:** Track T4-records-as-json's next incomplete slice is S16-journeys-attestations-align. Run /implement-slice S16-journeys-attestations-align 2026-06-27-conformance-foundation in a fresh session.
