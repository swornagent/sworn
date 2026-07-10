# Journal — S05-driver-registry

## 2026-07-10 — session 1 (implementer): design_review → in_progress

- Coach acknowledgement verified on the track branch: captain-proceed.md
  @41a402c, verdict PROCEED with five dispositions. Forward-sync merge
  914c0a4 brought the S06 R-04 spec amendment (proxy-aware dispatch is
  S06-owned) onto this branch before implementation started.
- Per captain-proceed.md disposition 3 / review.md pin 3, appended two
  Type-2 noted-default design_decisions to status.json at this transition:
  D2 prefix breadth (full chat-capable OAI-compat set + anthropic/ under
  oai-inprocess) and D3 choke-point rename in model.NewClient with
  utility-path spillover.
- Confirmed effort_complexity quadrant "grind" (high effort / low
  complexity) — the breadth is the fixture sweep the D3 spillover forces
  plus docs/help-text updates.
- Scope guard honoured: the AC-05 enumeration/dispatch proxy gap is owned
  by S06 R-04 (Coach-ratified); this slice does NOT touch
  internal/driver/inprocess/inprocess.go.

## 2026-07-10 — session 1 (implementer): in_progress → implemented

Implementation landed at bbb9ab2 (start_commit 20dc2dc). Decisions and
trade-offs a verifier may want context on:

- **Registry placement/API**: `internal/driver/registry` (package
  `registry`), `Default(cfg)` ≡ AC-01's `DefaultRegistry` — full divergence
  pack (four items, each with its forcing constraint) recorded in
  proof.json per captain-proceed.md disposition 2.
- **Warn surfaces (flag a)**: deprecation warning duplication ACCEPTED —
  `Registry.Resolve` warns via injectable `Warnf` (tests capture it);
  `NewClient`'s alias case warns to stderr for utility-path users. Deduping
  would need cross-package warn-state for an alias that dies next release.
- **Fixture sweep (D3 spillover)**: the chat/completions-shaped FromEnv
  proxy tests (UsesProxy/BypassProxy/ProxyDefaultHost/ProxyOverrideWarns/
  InsufficientCredits/NoCredsUnchanged + the invalid-base-URL table case)
  migrated `openai/gpt-4.1` -> `openai-completions/gpt-4.1` so each test
  keeps exercising the wire format it was written for; a NEW
  TestFromEnvProxyOpenAIIsResponses pins the re-keyed proxy branch
  (openai/ + alias -> OpenAIResponses). Notably TestFromEnvBypassProxy had
  begun dispatching to the REAL api.openai.com mid-sweep (BASE_URL override
  only applies to *OAI) — the migration restored hermeticity.
- **openai-completions key check**: explicit `SWORN_OPENAI_API_KEY` case in
  FromEnv (the generic default would demand SWORN_OPENAI_COMPLETIONS_API_KEY,
  which nothing sets and swornProviderConfig would not read into OpenAIKey).
  The `openai` case keeps its pre-existing default-path behaviour — no
  silent acceptance-widening to OPENAI_API_KEY on the direct leg.
- **Fused-line hazard (flag d)**: fixed both pre-existing fused comment
  lines in touched files (config.go proxy comment, provider.go
  ProviderConfig closing brace); fused-line grep + gofmt -l + go vet clean;
  full `go test -timeout 300s ./...` green (45 packages, fresh cache).
- **Gates**: `sworn verify` (model-backed, claude-cli/sonnet — the only
  keyless route in this environment) -> PASS, captured in proof.json.
  `sworn llm-check -type ac-satisfaction` could not dispatch (no model
  configured in env); no `sworn coverage` verb exists on this branch —
  manual AC-to-test cross-check recorded in proof.json divergence[].
- **index.md** re-rendered via `sworn render 2026-06-28-driver-contract`
  (board state, S05 implemented).

State: implemented. Next: fresh-context `/verify-slice S05-driver-registry
2026-06-28-driver-contract` (Rule 7).
