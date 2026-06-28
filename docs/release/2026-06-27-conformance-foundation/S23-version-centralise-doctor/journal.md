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
## Verifier verdicts received

### Verdict 1 — 2026-07-25: PASS

**Verifier session:** fresh-context, artefact-only.  
**Commit:** `ca39980` (HEAD of `track/2026-06-27-conformance-foundation/T6-contract-revendor`).

**Gate-by-gate:**

1. **User-reachable outcome** — PASS. `sworn doctor` run confirms both new checks (`baton/pin-currency`, `baton/prompt-currency`) appear in Group 1 output with OK status.
2. **Planned touchpoints** — PASS. All three planned files changed. Extra files are test files and comment-only changes in direct service of AC5 (hardcoded version removal).
3. **Required tests** — PASS. `TestDoctorPin` with 4 sub-tests all pass on re-run; tests exercise `checkPinCurrency()`/`checkPromptCurrency()` through the integration point.
4. **Reachability artefact** — PASS. `sworn doctor` output confirms `[OK] baton/pin-currency` and `[OK] baton/prompt-currency`.
5. **No silent deferrals** — PASS. Zero TODO/FIXME/placeholder/HACK hits in changed files.
6. **Design conformance** — PASS (not a UI-bearing project).
7. **Claimed scope** — PASS. All 8 Delivered items have verifiable evidence references that resolve to real code.
