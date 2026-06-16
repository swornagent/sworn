# Design TL;DR — S02-oai-model-client

## §1. User-visible change

`sworn verify --verifier-model openai/gpt-4.1` (or any `provider/model` pair) produces a real adversarial verdict from an OpenAI-compatible `/chat/completions` endpoint instead of the fail-closed `Unconfigured` stub. The customer sets `SWORN_<PROVIDER>_API_KEY` in their environment (BYO-key); the binary picks it up, dispatches the spec+diff+proof payload, and emits a JSON verdict with a non-zero `cost_usd` computed from the response `usage` fields. Without the env var, the command BLOCKs — same fail-closed contract as today, but now with a readable reason.

## §2. Design decisions not in spec (max 5)

1. **Model ID format `provider/model`** — the `--verifier-model` flag takes a slash-separated identifier (e.g. `openai/gpt-4.1`). The prefix maps to an env-var namespace (`SWORN_OPENAI_*`); the suffix is the model name sent in the API request body. This keeps the CLI surface small (one flag) while supporting multiple providers.

2. **Env var naming: `SWORN_<UPPER_PROVIDER>_API_KEY`** — canonical key; optional overrides `SWORN_<UPPER_PROVIDER>_BASE_URL` and `SWORN_<UPPER_PROVIDER>_MODEL` let the customer point at a proxy/gateway or override the model name without changing the CLI flag. Rationale: follows the twelve-factor "flat env vars per deploy" pattern; no config file needed for the MVP.

3. **`openai` default base URL** — when the provider prefix is `openai`, `SWORN_OPENAI_BASE_URL` defaults to `https://api.openai.com/v1`. This is the safe-hosted default: OpenAI is a trusted-jurisdiction provider; the customer must still supply their own key (the binary never ships with one). Any other provider requires an explicit `SWORN_<PROVIDER>_BASE_URL`.

4. **Cost from `usage` fields × per-model pricing table** — the OAI response `usage.prompt_tokens` and `usage.completion_tokens` are multiplied by a hardcoded per-model price map (USD per 1M tokens). A model not in the table gets a zero cost (still surfaced). Rationale: the spec requires `cost_usd` in the verdict; a static table avoids a runtime pricing-api call. The table is small (~5 entries) and versioned in the binary; S10 (benchmark) will make this data-driven.

5. **Provider registry as a single `FromEnv(modelID string) (Verifier, error)` constructor** — no plugin system, no dynamic registration. The constructor parses the model ID, reads env vars, validates (key required, URL well-formed), and returns a ready `*OAI` client. Rationale: one entry point, easy to test, easy to extend in S03 when the tool loop needs a different client shape.

## §3. Files I'll touch grouped by purpose

- **`internal/model/oai.go` (new)** — the `OAI` struct implementing `Verifier`: builds the `/chat/completions` POST, unmarshals the response, computes `cost_usd`. Pure `net/http` + `encoding/json`.
- **`internal/model/oai_test.go` (new)** — table-driven tests against an `httptest` fake server: PASS reply, FAIL reply, HTTP 500, timeout, garbled JSON, missing `usage` block. Each case asserts verdict + exit code.
- **`internal/model/config.go` (new)** — `FromEnv(modelID string) (Verifier, error)`: parses `provider/model`, reads `SWORN_<PROVIDER>_*` env vars, returns an `*OAI` (or a descriptive error).
- **`cmd/sworn/main.go` (edit)** — replace the `// Verifier left nil` line with `model.FromEnv(*mdl)` so `--verifier-model` actually wires a real client. Nil/empty model flag stays `Unconfigured` (backward-compatible).

## §4. Things I'm NOT doing

- Streaming responses — out of scope per spec.
- The agentic tool loop (function calling, multi-turn) — that's S03.
- A config file or `sworn init` — env vars only for this slice. S08 (`init` + `config`) will add file-based config.
- Per-model pricing API — static table only.
- Anthropic or non-OAI-compatible providers — the `provider/model` prefix convention leaves room, but the client only speaks `/chat/completions`. Other API shapes are deferred.

## §5. Reachability plan

**Artefact**: `sworn verify --spec <tmp/spec.md> --diff <tmp/diff> --verifier-model openai/gpt-4.1-mini` against a live endpoint (or an `httptest` fake server started by a smoke-test script). The JSON output on stdout will show `"verdict": "PASS"` (or `"FAIL"`) with `"cost_usd" > 0`. Documented in `proof.md` as a manual smoke step with the exact command and captured output. (E2E via a fake server is the unit test; the manual smoke is the reachability artefact proving the real wire path works end-to-end.)

## §6. Open questions for the Coach