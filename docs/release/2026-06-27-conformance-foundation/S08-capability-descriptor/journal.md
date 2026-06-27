# Journal — S08-capability-descriptor

## 2026-06-28 — Implementation session

**State:** planned → in_progress → implemented

### Decisions

1. **Capability type as bitmask:** Used `type Capability uint` with `iota` bit-shifting
   (`CapVerify = 1 << iota; CapChat`). This is extensible — adding `CapStream`,
   `CapEmbed`, etc. does not change existing return values.

2. **CapabilityProvider interface lives in `client.go`:** Co-located with the
   `Verifier` interface and `Capability` type so all three are in a single file.
   This avoids import loops (the interface is used by both `model` and `run` packages).

3. **Chat capability gate in `newAgentFromModel`, not `FromEnv`:** The gate lives
   in the run loop, not the model resolution layer. Rationale: `FromEnv` is used
   for both verifier and implementer model resolution; the Chat requirement is
   specific to the implementer role. The verifier can use any driver.

4. **Error message uses provider prefix, not full model ID:** The spec AC says
   `"driver anthropic does not support Chat"`. The provider prefix is extracted by
   splitting on `/` — this matches the spec's exact error format.

5. **Registry includes OAI-compat providers:** The `registry.go` includes all
   providers from `NewClient` (deepseek, groq, mistral, openrouter, cloudflare,
   github, vertex) — they all route through the `OAI` struct and therefore all
   have `CapVerify | CapChat`. This accurately reflects runtime capabilities.

### Divergence

- **env.go has no driver:** The spec's planned touchpoints list `internal/model/env.go`
  for adding `Capabilities()` boilerplate. However, `env.go` contains only package-level
  utility functions (`LoadDotEnv`, `loadFile`) — no `Env` struct exists. The
  compile-time interface assertion in the test still covers every actual driver type.
  No acceptance check is affected.

### Deferrals

None. All spec scope is implemented.

### Test coverage

- `internal/model/capabilities_test.go`: 3 subtests (AllDrivers, ChatBit, InterfaceAssertion)
- `internal/run/capabilities_test.go`: 3 subtests (reject-no-chat, reject-zero-caps, accept-chat)
- All existing tests continue to pass.
- `go vet` clean.
## 2026-06-28 — Verifier verdicts received

### Verdict 1 (2026-06-28 ~immediate)

**PASS**

Slice: `S08-capability-descriptor`
Verified against: `0549f1f`
Verifier session: fresh, artefact-only

All six gates passed:
1. **User-reachable outcome** — `newAgentFromModel` is wired into `sworn run` via `opts.NewAgent = newAgentFromModel` in `run.go:108` and `slice.go:115`. Capability gate fires before agent assertion.
2. **Planned touchpoints** — `env.go` (no driver struct) and `config.go` (spec-optional) not changed, explained in Divergence. `ollama.go` and `openai_responses.go` added to satisfy AC1 "ALL drivers" — minor touchpoint-list gap, not material.
3. **Required tests** — `capabilities_test.go` (model) + `capabilities_test.go` (run) both exist, exercise the integration point, all pass.
4. **Reachability artefact** — test command exits 0; integration test exercises `newAgentFromModel` capability gate.
5. **No silent deferrals** — "deferred" comments in anthropic/cli/ollama are explanatory, not deferrals.
6. **Claimed scope** — all 7 Delivered items verified against live code.

Next step: `/implement-slice S09-error-kind-consumption 2026-06-27-conformance-foundation`
