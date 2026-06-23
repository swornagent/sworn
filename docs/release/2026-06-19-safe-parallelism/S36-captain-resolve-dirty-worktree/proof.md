# Proof bundle — S36-captain-resolve-dirty-worktree

## Scope

Captain auto-resolves dirty track worktrees at clean-worktree gates instead of paging the Coach. Adds `resolve-dirty-worktree` function to `captain.md` with commit-by-default rule, detector contract, and prescriptive journal record format.

## Files changed

```
docs/release/2026-06-19-safe-parallelism/S36-captain-resolve-dirty-worktree/status.json
internal/prompt/captain.md
internal/prompt/prompt_test.go
```

## Test results

```
=== RUN   TestVerifier_NonEmpty
--- PASS: TestVerifier_NonEmpty (0.00s)
=== RUN   TestVerifier_ContainsVerdictContract
--- PASS: TestVerifier_ContainsVerdictContract (0.00s)
=== RUN   TestVerifier_NotOldPlaceholder
--- PASS: TestVerifier_NotOldPlaceholder (0.00s)
=== RUN   TestVerifier_ContainsInconclusive
--- PASS: TestVerifier_ContainsInconclusive (0.00s)
=== RUN   TestImplementer_NonEmpty
--- PASS: TestImplementer_NonEmpty (0.00s)
=== RUN   TestPlanner_NonEmpty
--- PASS: TestPlanner_NonEmpty (0.00s)
=== RUN   TestCaptain_NonEmpty
--- PASS: TestCaptain_NonEmpty (0.00s)
=== RUN   TestVerifyStateless_NonEmpty
--- PASS: TestVerifyStateless_NonEmpty (0.00s)
=== RUN   TestVerifyStateless_StatelessMarkers
--- PASS: TestVerifyStateless_StatelessMarkers (0.00s)
=== RUN   TestVerifyStateless_NotAgenticVerifier
--- PASS: TestVerifyStateless_NotAgenticVerifier (0.00s)
=== RUN   TestCaptain_ResolveDirtyWorktree
--- PASS: TestCaptain_ResolveDirtyWorktree (0.00s)
=== RUN   TestBatonVersion_NonEmpty
--- PASS: TestBatonVersion_NonEmpty (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/prompt	(cached)
```

```
go build ./...
BUILD: PASS
```

## Reachability artefact

**Test:** `TestCaptain_ResolveDirtyWorktree` verifies the embedded `Captain()` prompt contains:
- `resolve-dirty-worktree` function name
- `commits the work by default` (commit-by-default rule)
- `Discard only if clearly wrong` (discard guard)

**Walkthrough:** The `resolve-dirty-worktree` function in `captain.md` specifies the complete resolution path. Tracing from a dirty worktree at a clean-worktree gate:

1. **Detector** fires: `git -C <wt> status --porcelain` returns non-empty after filtering (tracked changes or untracked files within touchpoints). Non-touchpoint artefacts like a stray `sworn` binary are filtered out per spec Risk 2.

2. **Captain loads inputs:** the filtered diff + `status.json` touchpoints.

3. **Classification:** each dirty file is classified against the decision rule:
   - Tracked modification within touchpoints → commit
   - Untracked file within touchpoints → commit
   - Stray build artefact → discard (`git checkout --` or `git clean -fd <path>` scoped)
   - Accidental mass-deletion outside touchpoints → discard
   - Touchpoint-external edit with no coherent intent → discard
   - Otherwise → commit

4. **Commit/dispatch:** `git add` + `git commit -m "chore(...): auto-commit dirty worktree — <characterisation>"` + push. Or discard with per-file rationale.

5. **Journal record:** appends to the slice's `journal.md` with impacted files, diff characterisation, decision, files committed/discarded, and rationale.

6. **No Coach page:** the Coach is informed via the durable journal note. The only escalation case is a genuinely ambiguous diff (plausible work mixed with destructive changes, unclear split).

## Delivered

- **AC1:** `captain.md` defines a `resolve-dirty-worktree` function with commit-by-default rule and journal record. ✅ `internal/prompt/captain.md` lines 362–444.
- **AC2:** Contract states Coach is NOT paged for a dirty worktree except in genuinely-ambiguous case. ✅ `captain.md` line 362–366 and escalation rule in Procedure step 5.
- **AC3:** Deterministic detector contract specified: `git status --porcelain` filtered to tracked changes + touchpoint-scoped untracked files. ✅ `captain.md` "Detector contract" subsection, lines 370–381.
- **AC4:** Journal-record format specified (impacted files + diff characterisation + decision + rationale). ✅ `captain.md` "Journal record format" subsection, lines 426–436.
- **Test:** `TestCaptain_ResolveDirtyWorktree` verifies embedded prompt contains function name, commit-by-default rule, and discard guard. ✅ `internal/prompt/prompt_test.go` lines 106–117.
- **Build:** `go build ./...` passes.

## Not delivered

None. All spec acceptance checks are satisfied.

## Divergence from plan

None. The implementation follows the design.md with the three Coach-directed mechanical fixes applied:
- Pin 1: Detector contract includes filtered detection (tracked changes + touchpoint-scoped untracked files only), aligning with spec Risk 2.
- Pin 2: `design_decisions` added to `status.json` (4 Type-2 decisions).
- Pin 3: `internal/prompt/prompt_test.go` added to `planned_files`.