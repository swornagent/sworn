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