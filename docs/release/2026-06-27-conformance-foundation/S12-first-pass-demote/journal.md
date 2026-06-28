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
## Verifier verdicts received

### Verdict — 2026-07-25T03:30:00Z — BLOCKED

**Result:** BLOCKED

**Reason:** Drift-gate forward-merge of `release-wt/2026-06-27-conformance-foundation` into `track/2026-06-27-conformance-foundation/T3-agentic-verifier` produced a code conflict on `internal/model/oai.go`. This is a touchpoint-matrix violation (track-mode invariant 4): T2-model-layer (via release-wt) and T3-agentic-verifier's ancestry both modified the same file. Release-wt carries S08 (`Capability` type + `CapabilityProvider` interface) and S10-agentic-chat-anthropic (`Chat()`), while the track carries S10-provider-foundation, S39 (OpenAI responses), and S27 parallel-dispatch — all touching `internal/model/oai.go`. The planner must re-group or sequence these tracks.

**Proposed spec.md amendment:** Add `depends_on: [T2-model-layer]` to track T3-agentic-verifier in `docs/release/2026-06-27-conformance-foundation/index.md`, and re-materialise the T3 worktree from a base that includes T2-model-layer's merged changes. This ensures the track inherits `internal/model/oai.go` changes from T2-model-layer before layering its own.

**Next step:** `/replan-release 2026-06-27-conformance-foundation`
