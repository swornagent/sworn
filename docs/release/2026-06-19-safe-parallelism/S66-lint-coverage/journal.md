---
title: 'Implementation journal — S66-lint-coverage'
---

# S66-lint-coverage — Implementation journal

## 2026-06-25 — Initial implementation

### State transition: planned → in_progress → implemented

**Implementer session.** Ported `bin/release-coverage.sh` from bash to Go as `sworn lint coverage` in `internal/gate/coverage.go`.

### Decisions

1. **Package location: `internal/gate/`** — placed alongside `trace.go` (S65-lint-trace). Both are lint gates that read spec.md and produce structured reports. Same package, same test patterns, same `style` import for output formatting.

2. **force-add coverage.go**: `.gitignore` has `coverage.*` glob (line 10) targeting coverage output files (coverage.out, coverage.html). This glob also matches `coverage.go`. Force-added with `git add -f`. The naming follows the existing convention (`trace.go`, `trace_test.go`) and the planned touchpoints. Not renaming — renaming would deviate from the spec and create inconsistency. Tracked in journal for verifier context.

3. **CamelCase splitting**: Go test names use CamelCase (`TestValidateInputFields`), which tokenises as a single word. Added `splitCamel()` to break CamelCase identifiers into subwords before lowercasing and keyword matching. Without this, Go test names would never match AC keywords. TS/Python test names use natural language phrases that tokenise correctly without splitting.

4. **`describe()` exclusion**: The TS regex initially captured `describe()` blocks as test functions. `describe()` is a grouping/scope construct in Vitest/Jest, not a test — only `it()` and `test()` should count. Removed from `reTSTest`.

5. **Keyword matching strategy**: Simple token-overlap scoring (not semantic). Each AC text is tokenised, each test name is tokenised, and the intersection count is the score. The test function with the highest score wins. Candidates are reported in descending score order for uncovered ACs. This is deliberately mechanical — the LLM check (S70) handles semantic validation.

6. **`--base` flag**: Accepts a git ref for the diff base, defaulting to `start_commit` from `status.json` or `release-wt/<release>`. This matches the `lint deps` pattern.

### Trade-offs

- **Self-referencing ACs**: S66's own test function names (e.g. `TestRunCoverage_FullCoverage_Go`) use different vocabulary than the AC text (e.g. "maps every AC to a test"). The keyword matching correctly reports all 4 ACs as uncovered for this slice. The unit tests DO cover every AC via direct assertion; the coverage map is a supplementary lint. For downstream slices consuming this tool, the AC↔test keyword matching will be more natural.
- **No GitHub issue filed** for the `.gitignore` collision — it's a one-off that this slice's file name happens to match the `coverage.*` glob. The glob serves its intended purpose (ignoring `coverage.out` etc.) and should not be narrowed.

### Out-of-scope discoveries

None requiring tracking. All ACs are delivered.

## Verifier verdicts received

### 2026-06-25 — Verdict: PASS

**Verifier session** (fresh context, artefact-only). All six verification gates walked.

- Gate 1: `sworn lint coverage` wired in `cmdLint()` dispatch (line 49), reachable as `sworn lint coverage --slice <id> --release <name>`.
- Gate 2: All 3 planned touchpoints match actual changed files. Minor divergences explained in proof.md.
- Gate 3: All 16 unit tests re-run and passing. Tests exercise the `RunCoverage()` integration point.
- Gate 4: Reachability artefact exercised — binary builds and produces coverage map (3/4 matched, AC-04 uncovered — expected per proof.md due to keyword-matching vocabulary gap; exit-code behavior validated by unit tests).
- Gate 5: No TODO/FIXME/placeholder/deferred markers in any changed file.
- Gate 7: All 4 ACs have evidence references in proof.md and are satisfied by the code.

**Note**: `status.json` field `start_commit` (`2d9ec30`) is stale — not an ancestor of the current branch HEAD. The correct ancestor is `99571af`. The stale value did not affect the diff (git resolved via common ancestor) but should be corrected on the next implementer re-entry.