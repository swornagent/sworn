# Journal — S42-implement-step-timeout

## Session 1: implementation (2026-07-06)

### State transitions

- `design_review` → `in_progress` (Coach approved design with 3 apply-inline pins, Captain PROCEED verdict)
- `in_progress` → `implemented` (all acceptance checks pass, first-pass green)

### Decisions

1. **Config tier (Pin 1):** Added `ImplementerConfig { Timeout string }` to `Config` in `internal/config/config.go` and `config.ResolveImplementTimeout()` with precedence flag > env > config > default (15m). Parses duration strings via `time.ParseDuration`. Added `internal/config/config.go` to `planned_files`.

2. **Timeout wrapping in RunSlice:** Added per-attempt `context.WithTimeout` wrapping `implement.Run`. Resolves timeout at start of `RunSlice`: 0 → default constant, negative → no timeout, positive → as-is. Each iteration defers `cancel()` to prevent timer leak.

3. **DeadlineExceeded detection:** Uses `errors.Is(err, context.DeadlineExceeded)` to emit a distinct stderr message (`"implement attempt N timed out after <d> — escalating"`). The escalation path (continue to next model, or fail-closed on last attempt) is identical to other implementer errors.

4. **Error message for exhaustion:** Updated to `"RunSlice: implementer failed after N attempts (last error: ...). Escalate to human."` to match spec's "escalate to human" requirement.

5. **Design decisions:** Populated `design_decisions` in `status.json` with all 5 §2 decisions using S41's pattern (Type-2 stake class).

### S44 forward-compatibility (Pin 3)

`context.DeadlineExceeded` is a sworn-internal signal, not a `model.Error{Kind}`. When S44 adds Kind-based routing (`internal/run/slice.go`), `DeadlineExceeded` falls through to the existing "escalate to next model" path — the error does not carry a Kind, so S44's Kind switch won't match it.

### Trade-offs

- The `DefaultImplementTimeout` constant lives in `internal/config/config.go` (not `slice.go` as originally designed) because the config package is the natural home for a configurable default.
- Flag resolution uses `flag.Duration` for the `--implement-timeout` flag, which parses Go duration strings natively. The env and config tiers use `time.ParseDuration` for parity.
- The `time` import was removed from `cmd/sworn/run.go` — `fs.Duration` returns `*time.Duration` without requiring an explicit `time` import in the file.

### Tests

5 tests written in `internal/run/slice_test.go`:
- `TestImplementTimeoutEscalates` — blocking fake on slot 0 → timeout → escalation to slot 2 → PASS
- `TestImplementTimeoutExhaustsToHuman` — all blocking → "Escalate to human" error
- `TestImplementTimeoutHappyPath` — quick agent within timeout → unaffected
- `TestImplementTimeoutZeroUsesDefault` — zero timeout → resolved to default (15m) → agent runs
- `TestImplementTimeoutNegativeNoTimeout` — negative timeout → no timeout → agent runs

No skeptic panel — runtime does not support subagent dispatch in this session.

### Open deferrals

Both deferrals carried forward from spec and acknowledged by Coach (2026-06-21):
1. `http.Client.Timeout` on `oai.go` — deferred to S39/T5
2. Agent-spawned OS subprocess killing — deferred; supervisor covers cross-session orphans