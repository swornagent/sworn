# Captain review — S16-ollama-driver
Date: 2026-06-24T08:20:47Z
Design commit: cfffb16b7b0546edf53bb27ccd8a51edf4ba2621

## Pins

1. [mechanical] §3/§4 — `provider_test.go` missing from file plan; existing `TestNewClient_Ollama` will fail
   What I observed: The design §3 lists three files: `ollama.go`, `ollama_test.go`, `provider.go`. But `internal/model/provider_test.go` contains `TestNewClient_Ollama` (lines 57-82) which asserts `v.(*OAI)` and `BaseURL == "http://ollama.local:11434/v1"`. After replacing the `case "ollama"` dispatch with `NewOllama()`, this test will panic on the type assertion (`*Ollama` is not `*OAI`) and the BaseURL assertion will fail (no `/v1` suffix). The spec AC says "All prior model tests still pass" — this test cannot pass without modification. Neither spec planned touchpoints nor design §3 list `provider_test.go`.
   What to ask the implementer: Add `internal/model/provider_test.go` to `planned_files` in status.json and to design §3. Update `TestNewClient_Ollama` to assert `*Ollama` type and the native host (no `/v1`). This is the same pattern as S15-oci-driver's `TestNewClient_OCI` replacement.

2. [mechanical] §2.2 — Design rationale misreads current `OllamaHost` semantics
   What I observed: Design §2.2 says "Currently `ProviderConfig OllamaHost` is the `/v1` OAI-compat base." But `ollamaHost()` (provider.go:71-76) returns `http://localhost:11434` with NO `/v1` suffix. The `/v1` is appended at dispatch time in the `case "ollama"` block (line 140: `strings.TrimRight(base, "/") + "/v1"`). So `OllamaHost` already stores the raw host; the design's stated semantic change ("flips from OAI-compat base URL to raw host") is a no-op — the field already holds the raw host. The `ProviderConfig.OllamaHost` comment (line 23) says `defaults to http://localhost:11434/v1` which is stale and misleading, but the actual runtime value is correct.
   What to ask the implementer: Correct design §2.2 rationale: `OllamaHost` already stores the raw host (no `/v1`); the change is removing the `/v1` append in the dispatch block, not changing the field's stored value. Also update the stale comment on `ProviderConfig.OllamaHost` (line 23) from `defaults to http://localhost:11434/v1` to `defaults to http://localhost:11434` — this is a doc fix, not a behavioural change.

3. [mechanical] §2.3 — `NewOllama` env-read is unreachable through `NewClient` path
   What I observed: Design §2.3 says "Reuse `ollamaHost()` in `NewOllama` when the host param is empty." But `NewClient` will call `NewOllama(model, pcfg.OllamaHost)`, and `pcfg.OllamaHost` is already populated by `ollamaHost()` in both `ProviderConfigFromEnv()` (provider.go:50) and the config.go `FromEnv()` (config.go:149). So `pcfg.OllamaHost` is never empty when arriving via `NewClient`. The `$OLLAMA_HOST` fallback in `NewOllama` is only reachable when called directly (not through `NewClient`). The spec AC says "`NewOllama("llama3.2", "")` returns `*Ollama` with Host = `http://localhost:11434`" and "`$OLLAMA_HOST` env var sets the host when `NewOllama` host param is empty" — so the spec explicitly wants `NewOllama` to handle the empty-host case independently. This is fine, but the design should acknowledge that the `NewClient` path always passes a non-empty host, making the env-read in `NewOllama` a direct-call convenience, not a dispatch-path necessity.
   What to ask the implementer: No code change needed — the spec's `NewOllama` signature is correct. Just clarify in a code comment that the `$OLLAMA_HOST` fallback handles direct construction (tests, standalone use); the `NewClient` path always supplies `pcfg.OllamaHost`.

4. [mechanical] §4 — Stale `ProviderConfig.OllamaHost` comment not addressed
   What I observed: Design §4 says "Not touching the `ProviderConfig.OllamaHost` field name or its env-var reader — the field stays, only the consumer changes." But the field comment (provider.go:23) says `// optional, defaults to http://localhost:11434/v1` which is already wrong (the default has no `/v1`) and will be more wrong after this slice (the consumer no longer appends `/v1`). Leaving a stale comment that references the old OAI-compat format is a documentation hazard.
   What to ask the implementer: Update the comment on `ProviderConfig.OllamaHost` to `// optional, defaults to http://localhost:11434` as part of the `provider.go` modification. This is a one-line doc fix in a file already being touched.

5. [mechanical] status.json — `design_decisions` absent from status.json
   What I observed: `status.json` has no `design_decisions` field. The designfit gate (Rule 9, S32) checks for `design_decisions` in status.json and fails closed when absent. This is the recurring pattern seen across S19/S21/S23/S48/S49/S50/S60/S61 in this release. The five §2 decisions in design.md are all Type-2 (local, reversible: constructor signature, field reuse, response parsing, error format) and should be recorded as such.
   What to ask the implementer: Add a `design_decisions` array to status.json with the five §2 decisions classified as `type: "type_2"`, each with a one-line rationale. This ensures `sworn designfit` passes before the Verifier runs.

## Summary

Pins: 5 total — 5 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: 1 (pin 1 — `provider_test.go` not in file plan; existing test will fail, blocking "all prior model tests still pass" AC)

## Smaller flags (not pins, worth one-line ack)

- The `ollamaHost()` helper is called from both `provider.go` (line 50) and `config.go` (line 149). Both paths populate `pcfg.OllamaHost`. The design correctly identifies reusing it; no action needed.
- The spec's "Deferrals allowed?" section says "Model pull / list: deferred post-R3." This is a scope deferral, not a Rule 2 deferral — it's in the spec itself. No tracking issue needed.
- S39-openai-responses-provider (planned) and S63-subscription-cli-driver (planned) are in the same track but don't touch `provider.go`'s ollama case. No touchpoint collision.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Clean design, follows the established driver pattern well. 5 pins, all mechanical, 1 critical:

1. **Add `provider_test.go` to file plan.** `TestNewClient_Ollama` (provider_test.go:57-82) asserts `*OAI` and `/v1` BaseURL — it will fail after the dispatch change. Add `internal/model/provider_test.go` to `planned_files` in status.json and design §3. Rewrite the test to assert `*Ollama` and native host (no `/v1`).

2. **Correct §2.2 rationale.** `OllamaHost` already stores the raw host (no `/v1`); `ollamaHost()` returns `http://localhost:11434`. The `/v1` is appended at dispatch time (line 140). The semantic change is removing the `/v1` append, not changing the field's stored value. Fix the design prose accordingly.

3. **Clarify `NewOllama` env-read purpose.** The `NewClient` path always passes `pcfg.OllamaHost` (non-empty). The `$OLLAMA_HOST` fallback in `NewOllama` handles direct construction only. Add a brief code comment noting this.

4. **Fix stale `OllamaHost` comment.** Update `ProviderConfig.OllamaHost` comment (provider.go:23) from `defaults to http://localhost:11434/v1` to `defaults to http://localhost:11434`. One-line doc fix in a file already being touched.

5. **Add `design_decisions` to status.json.** Five §2 decisions, all Type-2 (local, reversible). Record them as `type: "type_2"` with one-line rationales so `sworn designfit` passes.

§2 decisions 1-5 ack (all Type-2, consistent with prior driver slices). §6 questions: none. Flags: (a) `ollamaHost()` called from both provider.go and config.go — both paths correct, no action; (b) spec deferral for model pull/list is scope-level, not Rule 2.

Address pins 1-5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All five pins are apply-inline corrections (file-plan addition, doc fixes, comment updates, status.json field); no design re-check needed before code.
-->