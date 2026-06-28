# S12-first-pass-demote — Proof Bundle

## Scope

Demote the stateless LLM judge to a deterministic first-pass gate (`RunFirstPass`). Re-vendor `verifier.md` from canonical. The first-pass catches structural blockers before the agentic verifier runs; a PASS from the first‑pass does NOT drive state transitions.

## Files changed

Verifier diff base (start_commit..HEAD):
```
$ git diff --name-only df48e66f0cb8bfbb8e010cbfc689280547e9904b
docs/release/2026-06-27-conformance-foundation/S12-first-pass-demote/proof.md
docs/release/2026-06-27-conformance-foundation/S12-first-pass-demote/status.json
```

Full implementation diff (parent of start_commit..HEAD, 13 files in 2 commits):
```
$ git diff --name-only df48e66f0cb8bfbb8e010cbfc689280547e9904b^..HEAD
cmd/sworn/verify.go
docs/release/2026-06-27-conformance-foundation/S12-first-pass-demote/journal.md
docs/release/2026-06-27-conformance-foundation/S12-first-pass-demote/status.json
internal/bench/runner.go
internal/model/oai.go
internal/model/openai_responses.go
internal/prompt/VERSION.txt
internal/prompt/verifier.md
internal/run/run_test.go
internal/run/slice.go
internal/verify/concurrent_test.go
internal/verify/verify.go
internal/verify/verify_test.go
```
## Test results

```
$ go test ./internal/verify/... -v -count=1

=== RUN   TestConcurrentVerifySameInput
--- PASS: TestConcurrentVerifySameInput (0.00s)
=== RUN   TestConcurrentVerifyIndependentInputs
--- PASS: TestConcurrentVerifyIndependentInputs (0.00s)
=== RUN   TestRunAgenticPass
--- PASS: TestRunAgenticPass (0.00s)
=== RUN   TestRunAgenticFail
--- PASS: TestRunAgenticFail (0.00s)
=== RUN   TestRunAgenticBlocked
--- PASS: TestRunAgenticBlocked (0.00s)
=== RUN   TestRunAgenticUnparseableBlocks
--- PASS: TestRunAgenticUnparseableBlocks (0.00s)
=== RUN   TestRunAgenticEmptyChoicesBlocks
--- PASS: TestRunAgenticEmptyChoicesBlocks (0.00s)
=== RUN   TestFirstPass_Pass
--- PASS: TestFirstPass_Pass (0.00s)
=== RUN   TestFirstPass_PassDoesNotWriteState
--- PASS: TestFirstPass_PassDoesNotWriteState (0.00s)
=== RUN   TestFirstPass_Fail_ModelReplyIgnored
--- PASS: TestFirstPass_Fail_ModelReplyIgnored (0.00s)
=== RUN   TestFirstPass_Blocked_EmptySpec
--- PASS: TestFirstPass_Blocked_EmptySpec (0.00s)
=== RUN   TestFirstPass_Blocked_EmptyDiff
--- PASS: TestFirstPass_Blocked_EmptyDiff (0.00s)
=== RUN   TestVerifyRun_Blocked_MissingFile
--- PASS: TestVerifyRun_Blocked_MissingFile (0.00s)
=== RUN   TestParseVerdictPass
--- PASS: TestParseVerdictPass (0.00s)
=== RUN   TestParseVerdictFail
--- PASS: TestParseVerdictFail (0.00s)
=== RUN   TestParseVerdictBlocked
--- PASS: TestParseVerdictBlocked (0.00s)
=== RUN   TestParseVerdictInconclusive
--- PASS: TestParseVerdictInconclusive (0.00s)
=== RUN   TestParseVerdictUnparseableBlocks
--- PASS: TestParseVerdictUnparseableBlocks (0.00s)
=== RUN   TestBuildPayload
--- PASS: TestBuildPayload (0.00s)
=== RUN   TestFirstPass_OpenDeferrals
--- PASS: TestFirstPass_OpenDeferrals (0.00s)
=== RUN   TestFirstPass_UndeclaredMockFails
--- PASS: TestFirstPass_UndeclaredMockFails (0.00s)
... (all boundary-mock tests PASS)
PASS
ok  	github.com/swornagent/sworn/internal/verify	0.013s
```

## Reachability artefact

`go test ./internal/verify/... -v` exits 0.

```
$ diff internal/prompt/verifier.md ~/.claude/baton/role-prompts/verifier.md
(no output — files are identical)
```

```
$ grep -rn 'verify\.Run\b' --include='*.go' --exclude='*_test.go' . | grep -v 'RunFirstPass\|RunAgentic\|reqverify\.Run'
(no output — zero bare verify.Run references in non-test code)
```

## Delivered

- [x] `verify.Run()` renamed to `verify.RunFirstPass()` — all callers migrated (`grep` confirms zero bare `verify.Run` in non-test code)
- [x] `RunFirstPass()` is deterministic-only: checks spec non-empty, diff non-empty, no undeclared boundary mocks; returns BLOCKED/FAIL/PASS without model dispatch
- [x] `RunFirstPass()` PASS result does NOT write `state.Verified` to status.json — the function signature takes no statusPath, and only the agentic verifier (`RunAgentic`) drives state transitions
- [x] `RunFirstPass()` FAIL or BLOCKED short-circuits `RunAgentic()` in `RunSlice` — first-pass gate inserted between mock lint and agentic dispatch
- [x] `internal/prompt/verifier.md` content matches canonical `~/.claude/baton/role-prompts/verifier.md` (verified by `diff` — no output)
- [x] `internal/prompt/verifier.md` contains no references to `v0.4.2` or stale section headings (grep confirms)
- [x] Tests updated: `TestFirstPass_Pass`, `TestFirstPass_PassDoesNotWriteState`, `TestFirstPass_Fail_ModelReplyIgnored`, `TestFirstPass_Blocked_Empty*`, concurrent tests updated for deterministic semantics
- [x] `internal/bench/runner.go` updated to use `RunFirstPass`
- [x] `cmd/sworn/verify.go` default path uses `RunFirstPass`

## Not delivered

None. All in-scope items delivered.

## Divergence from plan

- `RunFirstPass` in `RunSlice` writes diff to a temp file (path-based API). The `Input.DiffPath` field expects a file path, not a string. Adding a `DiffContent` string field was considered but explicitly avoided to maintain caller compatibility as specified.
- The `Input` struct retains `Model` and `Verifier` fields for compatibility even though `RunFirstPass` ignores them.

## First-pass script output

```
$ ~/.claude/bin/release-verify.sh S12-first-pass-demote 2026-06-27-conformance-foundation
  PASS  slice folder exists
  PASS  spec.md present
  FAIL  proof.md missing (now created)
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section
  PASS  status.json is valid JSON
  PASS  worktree branch is current with release/v0.1.0 (no drift)
  PASS  1 file(s) changed vs diff base
  PASS  no dark-code markers in changed source files
  PASS  spec.md frontmatter is strict-YAML safe
```