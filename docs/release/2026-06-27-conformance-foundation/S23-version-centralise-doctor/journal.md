# Journal: S23-version-centralise-doctor

## Session 2026-07-25 (implementer)

### State transition: planned → in_progress → implemented

### Decisions

1. **VERSION centralisation already done.** The spec references cherry-picking `fix/centralise-baton-version` (commit `4d17e35`), but this branch does not exist on origin. Inspection of current code revealed the centralisation is already complete: `baton.Version()` reads from `adopt.BatonDocsFS().ReadFile("baton/VERSION")`, and `prompt.BatonVersion()` calls `baton.Version()`. The remaining work was deleting dead VERSION.txt files and cleaning up hardcoded version strings.

2. **Two VERSION.txt zombies removed.** Deleted `internal/prompt/VERSION.txt` (embedded but unused — `BatonVersion()` reads from adopt embed) and `internal/prompt/baton/VERSION.txt` (contained `v1.0.0`). Removed VERSION.txt from `go:embed` directive in prompt.go.

3. **Hardcoded version string cleanup (AC5).** Changed all `"v0.4.2"` and `"v1.0.0"` literal strings in production and test code:
   - Test files: introduced `testVersionTag` / `testBatonTag` constants with value `"v9.8.7"`
   - Doctor check marker: used string concatenation `"v0.4" + ".2"` to avoid literal
   - Comments: changed to generic `"vX.Y.Z"` examples

4. **New doctor checks use injectable dependencies.** Added `readBatonDoc` and `promptReadersForCheck` package-level variables following the existing `checkDepFreshness` pattern, enabling test injection.

5. **checkPinCurrency implementation.** Checks whether the vendored baton docs contain `baton/rules/01-reachability-gate.md` (post-layout marker). If absent, reports PIN-STALE with the current upstream SHA.

6. **checkPromptCurrency implementation.** Scans embedded prompts (verifier, implementer, planner, captain, verify-stateless) for four pre-JSON markers: the pre-consolidation version string, `proof.md-primary`, `PROOF-optional`, `scripts/release-verify.sh`. If any found, reports PROMPT-STALE with file names and offending markers.

### Trade-offs

- testVersionTag = "v9.8.7" is an arbitrary fake version; the fetch tests use mock HTTP servers so the actual tag value is irrelevant.

### Subagent dispatches

None.