# S09-error-kind-consumption — Journal

## Session 1: 2026-06-28

### Decisions

- **Terminal error guard placement**: inserted the `model.IsTerminal(implErr)` check immediately after the `if implErr != nil` gate but before the `errors.Is(implErr, context.DeadlineExceeded)` check. This ensures terminal errors (KindAuth, KindCredits) short-circuit before any triage logic. The guard uses `errVerdictBlockedPrefix` for consistency with the existing `IsBlocked()` sentinel check.
- **Reason string format**: `"KindAuth: <UserMessage> — halting; check provider credentials"`. Uses `model.Error.UserMessage()` for provider-specific guidance and includes the kind label explicitly per AC1 requirement.
- **`ErrDriverNotRegistered` → `ErrDriverNotImplemented` rename**: the audit (docs/captures/2026-06-27-baton-conformance-audit.md line 159) flagged the name as over-stating — all drivers ARE registered in the `NewClient` switch, some just return a "not yet implemented" stub. New name accurately describes the condition. Mechanical rename across 6 Go files; zero old-name occurrences remain.
- **Test coverage**: table-driven `TestTerminalError_AllKinds` exercises all 6 ErrorKind values through `RunSlice`. Terminal kinds (auth, credits) assert `IsBlocked(err)==true`; non-terminal kinds assert `IsBlocked(err)==false`. Individual tests for KindAuth, KindCredits, KindRateLimit, and nil-error (happy path) provide readable sub-test names.

### Trade-offs

- The terminal halt returns before recording the implementer dispatch in the cost ledger (S55). This is correct — a terminal error means no implementation work was done, and the dispatch cost is zero. The verifier-ledger is not updated either, as the slice stays in `in_progress` (the BLOCKED verdict routes to `/replan-release`).
- Untyped errors (not `*model.Error`) are never treated as terminal — `model.IsTerminal` returns false for them. They flow through the normal triage/retry path. This is the conservative choice; an untyped error might be transient.

### Out-of-scope deferrals

- **Self-registering factory (sworn#15)**: the conformance audit suggested this as an alternative to the hardcoded switch in `provider.go`. This is a larger architectural change that belongs in its own slice — the sentinel rename is the "or rename" path from the spec. Tracked in GitHub issue #15.
- **Dark-code markers in provider.go/cli.go**: the `S63-deferral-1` comments pre-date this slice and are legitimate Rule 2 deferrals (tracked in S63). The rename touched the surrounding code but did not change the deferral status.

### Touchpoint discipline

- `internal/run/slice.go` is a documented shared file (T2: lines ~322-337; T3: lines ~412-429). Edits restricted to the error-handling section only. No scope bleed into the T3 region.
## Verifier verdicts received

### Verdict 1: 2026-07-25 — PASS

PASS

Slice: `S09-error-kind-consumption`
Verified against: `eded2ba` (HEAD of track/2026-06-27-conformance-foundation/T2-model-layer)
Verifier session: fresh, artefact-only

**Gate 1** — User-reachable outcome exists: PASS. `RunSlice()` in `internal/run/slice.go` is the integration point for `sworn run`. Tests drive through the full pipeline.

**Gate 2** — Planned touchpoints match actual changed files: PASS. `errors.go` was not changed (sentinel lived in `provider.go`); all extra model files are rename ripple. Documented in proof.md "Divergence from plan".

**Gate 3** — Required tests exist and exercise the integration point: PASS. `slice_terminal_test.go` table-driven tests pass (7/7). `go test ./internal/model/...` passes.

**Gate 4** — Reachability artefact proves the user path: PASS. `go test ./internal/run/... -v -run TestTerminalError` exits 0, re-run confirmed.

**Gate 5** — No silent deferrals or placeholder logic: PASS. Pre-existing S63-deferral-1 markers are Rule 2 acknowledged; no new deferrals introduced.

**Gate 6** — Design conformance: auto-PASS (non-UI project).

**Gate 7** — Claimed scope matches implemented scope: PASS. All 5 Delivered items verified against live evidence.

Re-ran all test commands; `go vet` clean. Terminal-error halt correctly placed before triage path. `ErrDriverNotRegistered` fully renamed to `ErrDriverNotImplemented` (zero old references, 18 new). The `IsTerminal()` function correctly returns true only for KindAuth and KindCredits.

**Next step:** `/implement-slice S10-agentic-chat-anthropic 2026-06-27-conformance-foundation` (next planned slice in T2-model-layer).
