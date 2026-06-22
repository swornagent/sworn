<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Clean design on a well-scoped slice. 6 mechanical pins to address inline:

1. **Voyage driver: honor cfg.BaseURL for test override.** Implement voyage driver to use `cfg.BaseURL || "https://api.voyageai.com/v1/embeddings"` (same pattern as Ollama's "default; overridable via base_url"). EmbeddingConfig.BaseURL already exists in config.go:75. Required for httptest unit tests and §5 integration test (step 3) to work without a live VOYAGE_API_KEY.
2. **AC6 test: key not in output.** Add a test (embed_test.go or cmd-level) that sets APIKeyEnv to a sentinel string and asserts the sentinel doesn't appear in error output or log lines. Mirror S23's TestAPIKeyEnvNotLeaked pattern.
3. **status.json: add design_decisions.** Populate the design_decisions array with the 5 §2 decisions before transitioning to in_progress. All are likely Type-2 (no Coach decisions needed); classify each with stake_class and rationale.
4. **§6.Q1 answered by spec.** Use LE binary (spec schema prescribes it). Add round-trip encode/store/retrieve/decode test (spec Risk #3 mitigation). Drop Q1.
5. **§6.Q2 answered by S01.** Driver name is "sqlite" — confirmed in internal/db/db.go:58. Drop Q2.
6. **§6.Q3 answered by spec.** Accept only `- [Title](file.md)` linked entries; plain bullets out of scope per spec. Drop Q3.

Flags (not pins): (a) CosineSimilarity + harness string literals are S24's public interface to S25 — define deliberately; (b) Voyage httptest fake response should match Voyage's actual JSON shape, not a generic OAI response.

§2 decisions 1–5 ack (all implementation-level choices, no escalation needed). §6 questions 1–3: resolved inline per pins 4–6 above.

Address pins 1–6 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 6 pins are apply-inline mechanical fixes; no Coach authority calls needed. Pin 1 (voyage httptest) is unambiguous: add one cfg.BaseURL fallback line in the driver.
-->
