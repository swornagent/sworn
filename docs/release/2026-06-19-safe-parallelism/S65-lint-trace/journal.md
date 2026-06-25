# S65-lint-trace — Implementation Journal

## 2026-07-15 — Implementation

### Design TL;DR

Port `release-trace.sh` from bash to Go as `internal/gate/trace.go`. The new package:
- Reads `intake.md` "What the human wants" section for needs (both explicit N-01 format and bold-label `**Label**` format)
- Reads `covers_needs` from each slice's `status.json`
- Checks: orphaned needs, invalid covers_needs refs, unclaimed coverage, EARS conformance, "see intake" references, vague-scope ACs, vague-scope in-scope items
- Produces structured JSON + human-readable output matching the bash script's style
- Exits 0 on PASS, 1 on FAIL

### Decisions

1. **New package `internal/gate/` instead of modifying `internal/rtm/`**
   - The existing `internal/rtm/` was built for the fidelity-layer release (S01-rtm-spine) and uses a different approach (tracing needs through AC text citations rather than covers_needs)
   - Creating a new package avoids breaking backward compatibility
   - The existing `cmd/sworn/lint.go` `cmdLintTrace` was swapped to call `gate.RunTrace` instead of `rtm.Build`

2. **Intake parsing: explicit N-01 format takes precedence over bold-label format**
   - If intake.md has `- N-01: description` lines, those are used directly
   - Otherwise, bold-label items in "What the human wants" section are auto-numbered as N-01, N-02, etc.
   - This matches the bash script's behavior

3. **covers_needs parsing: regex extraction from status.json**
   - Uses simple regex instead of full JSON unmarshal to avoid importing encoding/json for a single field
   - Handles empty arrays, single values, multiple values, and missing field

4. **EARS classification: Ubiquitous/When/While/Where/If/Complex**
   - Case-insensitive matching of EARS keywords
   - "shall" is the minimum bar for EARS conformance
   - Complex = 2+ EARS keywords present

### Trade-offs

- The regex-based `covers_needs` parser is simpler than encoding/json but won't handle nested structures. Given that status.json is machine-generated and the covers_needs array is always flat, this is safe.
- The Concrete term regex is conservative — it may miss some concrete terms (e.g. non-standard file extensions). The bash script has the same limitation.
- The existing `sworn lint ac` subcommand (which uses `internal/ears`) is NOT modified — both subcommands now do EARS checking independently, which is intentional (lint trace is the unified port of release-trace.sh).

### Out-of-scope discoveries

None.

### Test coverage

- 25 unit/integration tests in `internal/gate/trace_test.go` covering all check types
- 5 integration tests in `cmd/sworn/lint_trace_test.go` (existing, updated for covers_needs)
- Tested against the actual `2026-06-19-safe-parallelism` release: correctly identifies 465 violations (454 free-form ACs, 11 orphaned needs), matching the bash script's expected behavior
## Verifier verdicts received

### 2026-07-15 — verifier verdict — BLOCKED
BLOCKED
Slice: S65-lint-trace
Reason: The spec is internally inconsistent on the CLI entry point. "Entry point" section says "Invoked as `sworn lint trace`." (positional arg), but Acceptance check #1 specifies `sworn lint trace --release <name>`. Implementation (cmdLintTrace uses fs.Arg(0), no flag), proof.md reachability artefact, and all tests use positional form. The AC as written is not satisfied by the delivered code. This is a contract defect (spec inconsistency), not an implementation gap an implementer can close without changing the spec.
Proposed spec.md amendment: 
- Change AC #1 to: `sworn lint trace <release>` exits 0 on fully-traced release
- Update "User outcome" to remove "--release" (use positional)
- Update "Entry point" to be explicit: "Invoked as `sworn lint trace <release>` (positional release name, no --release flag)"
- Update reachability artefact description in proof.md if needed to match.
