# Captain review — S24-memory-engine
Date: 2026-06-21
Design commit: 54911e025f8b736630a4d6a436340ac4f94abb1c

## Pins

1. [mechanical] §5.step3 / §3.embed_voyage.go — Voyage driver endpoint hardcoded; httptest approach needs cfg.BaseURL fallback
   What I observed: §5 reachability step 3 says "voyage provider with a httptest listener." The voyage driver spec says endpoint is `https://api.voyageai.com/v1/embeddings` with no noted override, yet the required unit tests ("each driver tested against httptest.NewServer") require the endpoint to be interceptable. EmbeddingConfig.BaseURL already exists from S23 (config.go:75). The Ollama driver docs say "default; overridable via base_url" — voyage should do the same.
   What to ask the implementer: Implement the voyage driver to use `cfg.BaseURL` when non-empty, falling back to the hardcoded default — one conditional line. Without this, voyage unit tests cannot redirect to httptest and the §5 integration test (step 3) cannot work without a live VOYAGE_API_KEY. OAI-compat driver can serve as the integration-test provider if voyage override isn't added, but then AC1 (voyage provider) won't have a direct integration test path.

2. [mechanical] AC6 — No explicit "key not in output" test in embed_test.go plan
   What I observed: AC6 requires "api_key_env value is never logged or written to the index." The index schema has no api_key column (trivially satisfied). §2.5 covers when the key is read, but the design's embed_test.go plan doesn't include a "key value not in stdout/stderr/error messages" assertion. S23 set the precedent with TestAPIKeyEnvNotLeaked; the verifier will look for AC6 evidence.
   What to ask the implementer: Add a test case in embed_test.go (or cmd-level test) that sets the APIKeyEnv env var to a sentinel value (e.g. "TEST_KEY_SENTINEL_DO_NOT_EXPOSE") and asserts that the sentinel string does not appear in error output or log lines from the driver. This mirrors S23's TestAPIKeyEnvNotLeaked pattern.

3. [mechanical] status.json — Missing design_decisions field
   What I observed: status.json has no design_decisions array. S23's status.json had 5 classified decisions (two Type-1 with recorded human_decision). S24's design.md has 5 §2 decisions, but none are declared in status.json. `sworn designfit 2026-06-19-safe-parallelism` will fail on this slice. The S23 captain trial log entry records the same issue: "design_decisions absent from status.json."
   What to ask the implementer: Populate design_decisions in status.json with the 5 §2 decisions before transitioning to in_progress. Based on their nature (implementation-level choices with no Coach authority calls needed), all 5 are likely Type-2. Classify each accordingly.

4. [mechanical] §6.Q1 — LE binary already prescribed by spec schema
   What I observed: Q1 asks whether LE binary or JSON is preferred. The spec schema states: `embedding BLOB NOT NULL -- []float32 as little-endian IEEE 754`. The format is prescribed.
   What to ask the implementer: Drop Q1. Use LE binary (4 bytes per float, little-endian) as specified. Add the round-trip test called out in spec Risk #3 mitigation.

5. [mechanical] §6.Q2 — Driver name already confirmed in S01 code
   What I observed: Q2 asks if modernc.org/sqlite has been tested in S01. internal/db/db.go:15 uses `_ "modernc.org/sqlite"` and db.go:58 uses `sql.Open("sqlite", dbPath)`. Driver name is "sqlite". go.mod has `modernc.org/sqlite v1.52.0`.
   What to ask the implementer: Drop Q2. Use `sql.Open("sqlite", path)` — proven in S01 production code.

6. [mechanical] §6.Q3 — Parse rule already in spec
   What I observed: Q3 asks if MEMORY.md plain bullets (without links) should be parsed. The spec says: "reads MEMORY.md index file, parses `- [Title](file.md)` links, reads each linked file as one entry." Only linked entries are in scope.
   What to ask the implementer: Drop Q3. Accept only `- [Title](file.md)` linked entries; plain bullet text is out of scope per spec. Current MEMORY.md format in this repo is consistent with this.

## Summary

Pins: 6 total — 6 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: Pin 1 (voyage httptest approach fails without cfg.BaseURL fallback — voyage driver unit tests and §5 integration test can't run without a live API key)

## Smaller flags (not pins, worth one-line ack)

(a) CosineSimilarity function signature and harness string literals (discover.go) are cross-slice API surfaces consumed by S25. Define them deliberately — S25 imports them directly from the same package. No change needed now; just treat them as S24's public interface to S25.
(b) Risk #1 mitigation: "add a test against the exact Voyage response shape from their public docs" — the embed_test.go httptest response should match Voyage's actual JSON shape (`{"object":"list","data":[{"embedding":[...],"index":0}],"usage":{...}}`), not a generic OAI shape. Worth spelling out in the fake server response.
(c) S23's open deferral about cmd/sworn/main.go being a shared additive merge point (T3/T4) applies to S24 too — the build subcommand dispatch goes in main.go. Low risk (additive only), but the merge coordinator will need to handle it as with S23.

## Suggested ack reply
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
