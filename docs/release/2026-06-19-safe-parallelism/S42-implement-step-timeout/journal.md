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

`context.DeadlineExceeded` is a sworn-local signal, not a `model.Error{Kind}`. When S44 adds Kind-based routing (`internal/run/slice.go`), `DeadlineExceeded` falls through to the existing "escalate to next model" path — the error does not carry a Kind, so S44's Kind switch won't match it.

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
## Verifier verdicts received

### BLOCKED — 2026-07-07

**Actor**: verifier (`/verify-slice`)

**Verdict**: BLOCKED — forward-merge of `release-wt/2026-06-19-safe-parallelism` into `track/2026-06-19-safe-parallelism/T12-harness-hardening` conflicted on code files:
- `cmd/sworn/run.go`
- `internal/config/config.go`
- `internal/run/run.go`

The touchpoint matrix was wrong (track-mode invariant 4). This track (T12-harness-hardening) and the release-wt branch have concurrent edits to the same Go files. The planned_files for S42 (`internal/run/slice.go`, `internal/run/run.go`, `cmd/sworn/run.go`, `internal/config/config.go`) overlap with changes that have landed on `release-wt/2026-06-19-safe-parallelism` from other tracks (likely T3-commercial, in_progress with `internal/config/config.go` ownership).

**Proposed fix**: `/replan-release 2026-06-19-safe-parallelism` to re-group the conflicting slices so that T12's planned_files do not overlap with any in-flight release-wt changes.

**Next step**: `/replan-release 2026-06-19-safe-parallelism` (a blocked verdict routes to the planner, not back to the implementer).

## Planner correction — 2026-06-23 (replan resolving the BLOCKED verdict)

**Actor**: planner (`/replan-release`)

The BLOCKED verdict's framing was only half-right. Decomposing the forward-merge conflict
by *who actually touches each file while in-flight*:

- `internal/run/run.go`, `internal/run/slice.go`, `internal/config/config.go` conflicts are
  against **already-merged** T1/T3 work — ordinary integration the implementer resolves at
  `/implement-slice` Step 0, not a parallel-track race. Merged tracks cannot be re-grouped.
- The `config.go` conflict was **self-inflicted**: this implementation moved
  `DefaultImplementTimeout` into `internal/config/config.go`, deviating from the spec (which
  mandates a named constant in `internal/run/slice.go`). config.go is owned by merged T3 and is
  a planned touchpoint of T6/T16 — the deviation manufactured the collision.
- The one genuine in-flight collision is `cmd/sworn/run.go`, shared with S10 (T5, in_progress)
  and unrecorded for T12 in the touchpoint matrix.

**Resolutions ratified at replan:**

1. **`cmd/sworn/run.go` is now DOCUMENTED SHARED** (additive flag/wiring per track). T5 and T12
   stay parallel; `/merge-track` reconciles. (Chosen over a `T12 depends_on T5` edge — T12 is
   near-complete, T5 barely started.)
2. **Enforce the spec — no `config.go` touch.** The default stays a named constant
   `DefaultImplementTimeout` in `internal/run/slice.go`. The config-file timeout tier is
   **deferred** (Rule 2 card in the spec); precedence is now flag > env > default.

**`verification.result` cleared to `pending`; state → `failed_verification`.** On re-entry:
(a) `/implement-slice` Step 0 forward-merges `release-wt` to integrate the merged-T1/T3 run-loop
changes; (b) relocate `DefaultImplementTimeout` from `config.go` back to `slice.go`; (c) remove
the `--implement-timeout` config-file tier (keep flag + env + default); (d) re-prove. `start_commit`
is intentionally preserved (do not overwrite it on re-entry).
