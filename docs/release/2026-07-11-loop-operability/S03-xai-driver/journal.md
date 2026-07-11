# Journal — S03-xai-driver

## 2026-07-12 — Implementer session (design_review → in_progress → implemented)

### Session start
- Coach PROCEED was already committed (`captain-proceed.md`, verdict PROCEED,
  zero escalate pins). Verified it before writing code. Design gate satisfied.
- Track worktree already materialised at
  `sworn-worktrees/release-2026-07-11-loop-operability-T2-xai-driver`
  (branch `track/2026-07-11-loop-operability/T2-xai-driver`). S03 is the sole
  slice in T2, so no in-track sequential gate.
- `verification.result` was `pending` (not `blocked`) — Step 0b guard passed.

### Approach (matches design.md)
Additive provider registration — xAI (Grok) is OpenAI chat/completions-compatible,
so it rides the shared in-process OAI chat client. No bespoke SDK (ADR-0007).
Changes are one new `ProviderConfig` field, one `NewClient` prefix case, one entry
in `chatPrefixes` + its `keyFor` probe, one catalog provider def, and one pricing
map — each mirroring an existing sibling prefix.

### Coach pins applied (all 5, apply-inline)
1. **Catalog placement.** `xai` appended LAST in `catalogProviderDefs` (sorts
   after `openrouter`, not between mistral/ollama as the design table said).
   `TestCatalogProviderNames` `want` extended to 8 entries with `"xai"` last.
2. **Design decisions recorded.** D1–D4 written into `status.json`
   `design_decisions` (all Type-2, id/classification/description/rationale/
   acknowledged/acknowledged_by) at the `in_progress` transition.
3. **Structured-output proof.** `TestXAI_ChatStructured_ResponseFormat` drives
   `ChatStructured` through the NewClient-resolved xai client against an httptest
   server — proving our request build + response parse in strict `json_schema`
   mode. Live xAI strict-schema acceptance is doc-confirmed only
   (docs.x.ai structured-outputs); NO paid live dispatch was run. If a live wire
   quirk ever surfaces, D2's `StructuredToolCall` is the one-token contained
   fallback and the declared role set would be narrowed to match.
4. **Shared-package sequencing.** S02 (T1-conformance, planned) also touches
   `internal/model/`. No collision now; the `/merge-track` affected-package
   regression re-runs `go test ./internal/model/...` for the second lander. My
   hunks are confined to a new case/map/def per file.
5. **Role-honesty citation acknowledged** ([[project_model_layer_service_refactor]]):
   impl/verify/captain are honest for xai/ precisely because it rides the oai Chat
   client (OpenAI-compatible on that exact path). The shared `NewOAIChat` driver
   declares all three roles (`internal/driver/inprocess/inprocess.go:98`).

### Divergence from design
- **D4 / `swornProviderConfig()`.** The design text said add
  `XAIKey: os.Getenv("SWORN_XAI_API_KEY")` (SWORN_-only) to `swornProviderConfig()`.
  Implemented as `envOrAlias("XAI_API_KEY", "SWORN_XAI_API_KEY")` instead —
  matching the sibling `GoogleKey` line in that same function. Reason: `FromEnv`
  passes the pcfg from `swornProviderConfig()` into `NewClient`, which reads
  `pcfg.XAIKey`; a SWORN_-only read would let a canonical-`XAI_API_KEY`-only user
  pass the key-presence gate (which uses `envOrAlias`) but then dispatch with an
  empty key. Using `envOrAlias` makes canonical-only work end-to-end on the
  one-shot path, which is exactly D4's stated intent ("honour the canonical var
  on the one-shot path too"). Net: honest, correctness-preserving; the literal
  design line would have been a latent bug.

### Reachability (smoke, no live dispatch)
- `XAI_API_KEY=sk-… sworn capabilities` → oai-inprocess lists `xai/` in prefixes,
  roles `implementer,verifier,captain`, `available: yes — API keys present: xai/`.
- No key → `xai/` still listed, `available: no`.
- `sworn models --provider zzz` → "valid providers: …, openrouter, xai".

### Verification hygiene
- Newline-eating hazard grep on changed `.go` (fused `//`+code): clean.
- `gofmt -l` on all changed files: clean. `go vet`: clean. `go build ./...`: ok.
- Full `go test -count=1 -timeout 300s ./...`: all packages PASS.

### Out-of-scope / deferrals
- **grok-4.5 exact pricing snapshot.** The `xaiPricing` entry uses xAI's published
  Grok flagship API rate ($3/$15 per 1M, 2026-07-12 snapshot) — a real non-zero
  entry so `CostSource=pricing-table` not `unknown` (AC-04). Exact per-1M rate to
  be re-confirmed against x.ai/api pricing (grok-4.5 may postdate the flagship
  $3/$15 snapshot used here). Spec R-4. Tracked: sworn#99.

## 2026-07-12 — Verifier verdicts received

### PASS (fresh-context verifier, artefact-only)
All six gates passed against `track/2026-07-11-loop-operability/T2-xai-driver` @ 9c71410 (start 71b0030):
- Gate 1 (user-reachable outcome): xai/ resolves through the driver registry; `sworn capabilities` and `sworn models` surface it. Re-run live from the worktree binary.
- Gate 2 (touchpoints): all changed files within plan. Planned `cmd/sworn/capabilities.go` and `cmd/sworn/models.go` were not edited because both surfaces derive dynamically (`registry.Default(...).Drivers()` and `model.CatalogProviderNames()`) — the outcome is delivered without editing them; benign over-estimate, not a dropped deliverable.
- Gate 3 (tests exercise integration point): TestResolveXAIRoles (registry integration point, all three roles), TestCapabilitiesListsXAI (both key states), TestXAI_ChatStructured_ResponseFormat (ChatStructured via httptest, strict json_schema). All re-run green in fresh window.
- Gate 4 (reachability): cli-run evidence reproduced live — xai/ available with key, listed+unavailable without, `xai` in valid providers.
- Gate 5 (no silent deferrals): no markers in changed files; single deferral (exact grok-4.5 pricing) tracked to existing OPEN GitHub issue sworn#99. Minor nit: the `internal/model/xai.go` comment cites sworn#41 (which actually tracks claude-sonnet-5 pricing) — the authoritative deferral surfaces (proof.json/status.json) correctly cite sworn#99. Non-blocking.
- Gate 6 (design conformance): no design-fidelity config; non-UI backend slice — passes automatically.
- Gate 7 (claimed scope): each delivered item maps to real, verified code + tests.
- Divergence D4 (envOrAlias vs SWORN_-only) reviewed: sound realisation of the design's stated intent, consistent with the sibling GoogleKey line; correctness-preserving.
- Full `go test -count=1 -timeout 300s ./...` re-run: exit 0, all packages PASS. gofmt/vet clean; no newline-eating corruption.

Verdict: PASS. Slice → verified. T2-xai-driver has only this slice → track complete; next `/merge-track T2-xai-driver`.
