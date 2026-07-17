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

## 2026-07-17T11:04:16+10:00 — Automatic Coach acknowledgement and Captain PROCEED

- Under the Coach's standing instruction to orchestrate this release, the
  Captain's `PROCEED` verdict in `review.md` (commit `6665355`) is
  acknowledged. There are no `[escalate]` pins and no new Type-1 decision to
  seek.
- Apply pin 1 inline: bind the stricter tool-call policy exclusively to direct
  OpenRouter; prove proxy OpenRouter, unprofiled OAI, and Ollama reject before
  dispatch, and preserve existing DeepSeek forced-tool behavior.
- Apply pin 2 inline: validate relative, hostless, and non-HTTP(S)
  `SWORN_OPENROUTER_BASE_URL` values as pre-dispatch failures; prove proxy and
  other-provider endpoint isolation.
- Apply pin 3 inline: preserve canonical report parameters and S04
  requested/emitted equality as semantic authority, with no repair, fallback,
  retry, or synthetic report.
- The skipped design-review LLM check is a no-network limitation, not an
  AC-06 waiver. Only after deterministic evidence may S22 perform one direct
  `openrouter/z-ai/glm-5.2` `spec-ambiguity` proof; S20 remains blocked until
  a fresh S22 verifier PASS.
- Proceed to `in_progress` only in a fresh Implementer session. That session
  must stop at `implemented`; only a fresh S22 verifier certifies the slice.

## 2026-07-17T11:27:23+10:00 — Implementer transport checkpoint

- The first S22 red was the built `sworn llm-check` reachability test: direct
  `openrouter/z-ai/glm-5.2` correctly failed locally before this slice added a
  structured route. It used a synthetic key and a local test endpoint only.
- Direct construction now has a separate forced-tool route and exact named
  call policy. Proxy OpenRouter stays default-deny; unprofiled OAI and Ollama
  reject before dispatch; the existing DeepSeek forced-tool behavior remains
  a separate legacy path.
- `SWORN_OPENROUTER_BASE_URL` is validated only after direct construction and
  only after proxy routing has declined. Relative, hostless, and non-HTTP(S)
  values fail before dispatch; proxy and another provider do not inherit it.
- The immutable S22 diff base is `a09b0e46df465862d00469d4aef2a997442b3d5b`.
  No credentialed provider check has run yet; AC-06 remains gated on the full
  deterministic suite, vet, and build.
