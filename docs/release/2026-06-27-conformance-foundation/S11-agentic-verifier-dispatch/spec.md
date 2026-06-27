---
title: 'S11 — Agentic verifier dispatch from engine'
description: 'Add an agentic verification path that dispatches the real verifier.md role (test-re-running, live-repo) from the engine; make proof mandatory (fail closed when absent); set verifier_was_fresh_context honestly; fix Verification.Model to record verifier model, not implementer model; wire no-mock RunMock check.'
---

# Slice: `S11-agentic-verifier-dispatch`

## User outcome

When `sworn run` (or `sworn verify --agentic`) verifies a slice, it dispatches the real agentic `verifier.md` role via a `model.Chat()` call (not the stateless judge), which re-runs tests, reads live repo state, and returns PASS/FAIL/BLOCKED. `verifier_was_fresh_context` is set to `true`. `Verification.Model` records the verifier model, not the implementer model. The loop refuses to verify a slice with no proof bundle.

## Entry point

- `sworn run` engine path: `internal/run/slice.go` lines ~412-429 (the verify dispatch section)
- `sworn verify --agentic <slice-id>` CLI (new flag on existing `cmd/sworn/verify.go`)

## In scope

- `internal/run/slice.go` (T3 documented-shared section §412): replace the `verify.Run(ctx, in)` stateless judge call with a `verifyAgentic(ctx, opts, statusPath)` call that dispatches `verifier.md` via `model.Chat()`; the stateless judge is NOT called for the `verified` state transition (it is still used as a first-pass pre-flight in S12)
- **Proof mandatory**: before the agentic verifier dispatch, call `state.Read(statusPath)` and check that `proof_path` references a non-empty file; if absent/empty, return BLOCKED immediately with "proof bundle absent — fail closed"
- `internal/verify/verify.go`: add `RunAgentic(ctx, Input)` function that loads `internal/prompt/verifier.md`, builds the Chat message array (system = verifier role prompt; user = SPEC+DIFF+PROOF payload), calls `model.Chat()`, and returns the parsed verdict
- **verifier_was_fresh_context**: in the PASS path (slice.go lines ~415-430), set `st.Verification.VerifierWasFreshContext = boolPtr(true)` when the agentic path was taken; set `false` if the stateless judge is used (for future first-pass paths)
- **Verification.Model fix**: in the PASS path, replace `st.Verification.Model = implModelID` with `st.Verification.Model = opts.VerifierModel` (the verifier model from the run options)
- **No-mock wiring**: before the agentic verifier dispatch, call `gate.RunMock(specPath, openDeferrals)` and if it returns any violations, append them to the existing `open_deferrals` warning list (not BLOCK by itself, since the deferral path is the user's explicit choice; the block only fires if the no-mock violation is not declared as a Rule-2 deferral)
- `internal/gate/mock.go`: add entitlement/credits/subscription/keyless keywords to the mock-boundary detection pattern list (audit: "zero entitlement keywords")
- `cmd/sworn/verify.go`: add `--agentic` flag that routes to `RunAgentic()` instead of `Run()`

## Out of scope

- Demoting the stateless judge (S12 handles that, including its labeling as first-pass)
- Full tool-use in the agentic verifier (the verifier role gets Chat but not tool-call permission in this slice; test-re-running is via inline system prompt instruction to run tests and include output)
- Changes to `internal/run/run.go` or the orchestrator

## Planned touchpoints

- `internal/run/slice.go` (T3 documented-shared section §412 — verifier dispatch path only)
- `internal/verify/verify.go` (add RunAgentic function)
- `internal/gate/mock.go` (add entitlement keywords)
- `cmd/sworn/verify.go` (add --agentic flag)

## Acceptance checks

- [ ] WHEN `sworn run` enters the verify step for a slice that has no proof bundle (proof_path absent or file empty), THE SYSTEM SHALL return BLOCKED with reason "proof bundle absent — fail closed" before dispatching the verifier
- [ ] WHEN `sworn run` enters the verify step and proof exists, THE SYSTEM SHALL call `RunAgentic()` (not `Run()` the stateless judge) for the `verified` state transition
- [ ] WHEN `RunAgentic()` receives a PASS from the model, THE SYSTEM SHALL write `st.Verification.VerifierWasFreshContext = true` and `st.Verification.Model = opts.VerifierModel` to status.json
- [ ] `st.Verification.Model` in the written status.json MUST equal the verifier model ID (e.g. `claude-sonnet-4-6`), not the implementer model ID
- [ ] WHEN `gate.RunMock(specPath, openDeferrals)` detects an entitlement/credits/subscription mock boundary not in open_deferrals, THE SYSTEM SHALL append a warning to the run log (does not BLOCK unless undeclared)
- [ ] `internal/gate/mock.go` pattern list includes at minimum: "entitlement", "credits", "subscription", "keyless", "claude -p"
- [ ] `sworn verify --agentic S01-llm-interpreter` compiles and routes to RunAgentic()

## Required tests

- **Unit**: `internal/verify/verify_agentic_test.go` (new) — mock Chat() client; test PASS/FAIL/BLOCKED verdict parsing
- **Unit**: `internal/gate/mock_test.go` (extend existing) — assert entitlement/credits patterns trigger detection
- **Integration**: `internal/run/slice_test.go` — add scenario: no proof bundle → RunSlice returns BLOCKED before verifier dispatch
- **Reachability artefact**: `go test ./internal/verify/... ./internal/gate/... -v -run TestAgentic|TestMock` exits 0; `sworn verify --agentic --help` displays the flag

## Risks

- The verifier.md role prompt instructs the verifier to run tests; in the agentic path this means the Chat conversation must include the test output — if the model doesn't have tool-call access to actually run tests, it will simulate; true test re-running requires tool access (deferred to a future slice that adds tool-call support)
- The `verifier_was_fresh_context` field is currently `*bool` in status.json; ensure the schema reflects this (nullable)

## Deferrals allowed?

Yes — true test re-running via tool calls is deferred. Rule 2: Why = tool-call support in the verifier path requires agentic tool infrastructure not yet in scope for this slice; the model is instructed to run tests and include output, but this requires the human-run environment. Tracking = future "agentic tool-calls" slice. Acknowledged = Brad, 2026-06-27.
