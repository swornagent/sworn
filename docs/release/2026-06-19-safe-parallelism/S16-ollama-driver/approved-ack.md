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
