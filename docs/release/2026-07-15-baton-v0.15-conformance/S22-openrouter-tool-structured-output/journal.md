# S22-openrouter-tool-structured-output journal

## 2026-07-17T10:29:17+10:00 — Planned

- Coach selected `openrouter/z-ai/glm-5.2` for the pending provider-readiness
  path.
- Current `openrouter/` resolution remains fail-closed for structured generic
  checks, so S22 adds only a direct forced-function route with a local
  built-command fake-endpoint proof.
- Sworn hosted-proxy OpenRouter, Ollama, and every other provider remain
  structured-output unsupported. S04's canonical validation and requested/
  emitted identity gate remain authoritative.
- The S22 dedicated live `spec-ambiguity` check is explicitly tracked for after
  deterministic implementation evidence. A fresh S22 verifier PASS is required
  before the separate immutable S20 smoke; real Codex and Claude homes remain
  untouched.

## 2026-07-17T10:45:08+10:00 — Implementer design checkpoint

- Read the S22 contract, current T1 state, S21's verified transport boundary,
  the current model/gate/CLI seams, and the live direct-versus-proxy route
  construction. The track is clean and synchronized; no code, tests, proof
  bundle, or provider call was produced.
- Direct OpenRouter will be an explicit construction-time forced-tool route,
  not a shared provider mapping that proxy construction can inherit. Its model
  subpath and canonical tool parameters remain intact; proxy OpenRouter,
  Ollama, and unprofiled clients stay structured-output unsupported.
- The direct response policy will require exactly one named
  `emit_structured_output` call containing one JSON object. The existing
  generic canonical validation and S04 emitted-check equality remain after
  transport extraction; malformed output has no repair, fallback, retry, or
  second request.
- `SWORN_OPENROUTER_BASE_URL` is a direct-only, validated fake-endpoint seam.
  The later credentialed S22 ambiguity dispatch remains deferred to AC-06
  implementation evidence; S20 remains blocked, unmodified, and independently
  responsible for any later smoke.

## Handoff

- `design.md` records the direct/proxy selection, exact tool response policy,
  configuration seam, acceptance trace, deterministic reachability evidence,
  and deferred live-check boundary. Run a fresh Captain
  `/design-review S22-openrouter-tool-structured-output 2026-07-15-baton-v0.15-conformance`
  before implementation.
