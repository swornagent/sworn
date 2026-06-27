# S11-agentic-verifier-dispatch — Implementation Journal

## Session 2026-07-24: Implementation

### Decisions

1. **RunAgentic interface**: Accepts raw spec/diff/proof strings + `agent.Agent`, rather than file paths. The caller (RunSlice) reads files. This keeps the verify package file-I/O-free for the agentic path.

2. **Proof-mandatory gate**: Placed in RunSlice before any verifier dispatch. The spec requires proof.md present and non-empty. On absence, we BLOCKED immediately with state commit — no goto (avoided goto-over-declaration issue in Go).

3. **Model fix**: `st.Verification.Model` now records `opts.VerifierModel` instead of `implModelID`. This was a latent bug — the stateless path recorded the implementer model as the verifier model.

4. **VerifierWasFreshContext**: Set to `true` on PASS in the agentic path. Uses `*bool` (nullable) per the existing schema. Added `boolPtr()` helper.

5. **No-mock wiring**: `gate.RunMock()` is called before the agentic dispatch. Violations are informational only (logged to stderr). The gate's internal deferral check handles declared mocks.

6. **Keyword additions**: Added `credits`, `Credits`, `keyless`, `Keyless`, `claude -p` to both `internal/verify/verify.go` knownBoundaryPatterns and `internal/gate/mock.go` realInfraPatterns. The `claude -p` keyword requires the mock marker and keyword on the same line for `CheckBoundaryMocks` detection (substring-based).

### Trade-offs

- The agentic verifier does NOT have tool-call access in this slice (deferred per spec). The verifier role prompt instructs the model to run tests, but without tools it can only simulate/describe what it would do. True test re-running requires the future "agentic tool-calls" slice.

### Deferrals

- True test re-running via tool calls deferred. Why: agentic tool-call infrastructure not in scope for this slice. Tracking: future agentic-tool-calls slice. Acknowledged: Brad, 2026-06-27.

### State transition

`planned → in_progress → implemented`