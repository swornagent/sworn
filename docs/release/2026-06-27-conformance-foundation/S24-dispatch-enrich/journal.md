# S24-dispatch-enrich — Journal

## Session 1 — 2026-07-12 (implementer)

**State transition:** `planned → in_progress`

### Implementation plan
1. Add `DurationMS`, `InputTokens`, `OutputTokens`, `ModelIDConfirmed` fields to `state.Dispatch`
2. Change `Verifier` interface `Verify()` to return `(text, costUSD, inputTokens, outputTokens, error)`
3. Update OAI and Anthropic Verify() to return token counts
4. Update all other Verify() implementations to return (0, 0) for tokens
5. Update `verify.Run()` to capture tokens, measure duration, populate `verdict.Result`
6. Add fields to `verdict.Result` for token counts, duration, model-id-confirmed
7. Create public `model.PriceForModel()` for cost computation from tokens
8. Update dispatch-append call sites in `run/slice.go`
9. Extend `state_test.go` with round-trip tests

### Decisions
- Using int64 for token counts and duration_ms per spec
- Changing Verifier interface signature (breaking) per spec's "backward-compatible approach" guidance
- Will wrap verify and implement calls with time measurement for duration
- OAI ChatResponse gets `Model` field for confirmed model ID capture
