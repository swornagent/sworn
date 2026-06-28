# S12-first-pass-demote — Journal

## Session: 2026-07-25T02:30:00Z

### Decisions

1. **`Run()` renamed to `RunFirstPass()`** — all callers updated. The function is now purely deterministic (no model dispatch). It checks: spec non-empty, diff non-empty, no undeclared boundary mocks.
2. **Model dispatch removed from RunFirstPass.** The old `systemPrompt` (stateless judge) variable was removed. The `verifierRolePrompt` remains for `RunAgentic`.
3. **First-pass integrated into `RunSlice`** — runs between mock lint gate and agentic verifier dispatch. On FAIL/BLOCKED, short-circuits with an informative reason and prevents the agentic call.
4. **`internal/prompt/verifier.md` re-vendored** from canonical `~/.claude/baton/role-prompts/verifier.md`. VERSION.txt created to note the re-vendor commit reference.
5. **Tests updated:** renamed all `Run`→`RunFirstPass` in test files, removed model-dependent tests (`TestSystemPromptIsStatelessJudge`), added `TestFirstPass_Fail_ModelReplyIgnored` and `TestFirstPass_PassDoesNotWriteState`, updated concurrent test to use deterministic inputs.

### Trade-offs
- The `Input` struct retains `Model` and `Verifier` fields for caller compatibility but `RunFirstPass` ignores them. A future cleanup could split `FirstPassInput` from `AgenticInput`.
- `RunFirstPass` in `RunSlice` writes diff to a temp file (path-based API). Could be optimised by adding a `DiffContent` field to `Input`, but the spec explicitly says "function signature accepts Input for caller compatibility."

### Pre-existing test failures (not caused by S12)
- `TestRunSlice_FailNotifiesOnce`, `TestRunSlice_BlockedNotifies`, `TestRunSlice_NilNotifierNoOp` fail because the fake verifier returns unparseable output → BLOCKED rather than FAIL. These test failures pre-date S12.