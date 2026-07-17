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
  `/design-review S22-openrouter-tool-structured-output 2026-07-15-baton-v0.16-conformance`
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

## 2026-07-17T11:33:17+10:00 — AC-06 evidence block

- Focused deterministic tests, `go test ./...`, `go vet ./...`, and `make
  build` completed before the one authorized direct S22
  `openrouter/z-ai/glm-5.2` `spec-ambiguity` command was invoked with the
  immutable S22 start commit.
- The command's output was redirected to avoid retaining any provider
  diagnostic or model payload, then disposed. This execution channel returned
  no sanitized exit/result receipt. No exported `OPENROUTER_API_KEY` was
  present; no credential source was inspected or exposed.
- A clean AC-06 result therefore cannot be evidenced. Per the exactly-once
  constraint, no retry, fallback, repair, S20 activity, or real-home mutation
  is permitted. S22 is blocked short of `implemented` pending a human decision
  on how to recover the required non-secret receipt or authorize any future
  provider action.
- This is not a fresh verifier verdict: `verification.result` remains
  `pending` so a later S22 verifier is not misrouted through the planner-only
  blocked-verdict guard.

## 2026-07-17T17:51:30+10:00 — Coach-authorized S22 receipt recovery

- The Coach expressly ratified a narrow recovery after the advisory audit:
  add deterministic rejection of JSON `null` tool arguments and of a returned
  tool call whose `type` is not `function`. These are S22-only changes to the
  already-planned `internal/model/oai.go` and
  `internal/model/structured_test.go` touchpoints; the existing T1 code and
  immutable start commit `a09b0e46df465862d00469d4aef2a997442b3d5b` are
  preserved.
- The prior AC-06 invocation has no usable sanitized receipt and is neither a
  PASS nor a FAIL. It is not evidence of a completed proof and is not a fresh
  verifier result.
- After AC-07, AC-08, and every deterministic S22 gate pass, exactly one new
  direct `openrouter/z-ai/glm-5.2` `spec-ambiguity` proof is authorized. This
  is a Coach-authorized recovery action, not a silent retry.
- The only retained receipt fields are check identity, model ID, immutable
  start commit, process exit code, and PASS/FAIL/BLOCKED/UNPARSEABLE result.
  Raw provider/model output is private-temporary then destroyed, or never
  retained. There is no fallback and no further retry.
- The external evidence block is cleared by moving only S22 back to
  `in_progress` for a fresh Implementer. S20 remains blocked and untouched;
  it cannot resume without the one new receipt followed by a fresh S22
  verifier PASS.

## 2026-07-17T17:56:38+10:00 — Planner deterministic validation

- Re-rendered `index.md` from `board.json` and the current slice records.
  `jq` parsing and `git diff --check` passed.
- Passed the non-provider planner gates for the entire release: `sworn lint
  ac`, `sworn lint trace`, `sworn reqvalidate`, `sworn designfit`, and `sworn
  specquality`.
- No provider/model/LLM call was made. In particular, the future AC-06 direct
  proof and model-backed requirements checks were intentionally not run under
  this Planner session's scope; this is not a waiver of the one newly
  authorized S22 proof.

## 2026-07-17T20:47:05+10:00 — Planner material replan: native proof receipt

- The Coach ratified a material replacement of the previous shell-style
  receipt recovery. The raw output from the historical invocation remains
  destroyed and must not be sought or reproduced. Its retained safe facts are
  now committed as attempt 1: release, slice, spec-ambiguity,
  openrouter/z-ai/glm-5.2, immutable S22 start, unavailable exit semantics,
  UNPARSEABLE, and receipt_failure.
- S22 now owns a separate native proof-receipt facility, not LLMCheckReport:
  validate the exact release/slice/check/model/start binding, atomically reserve metadata before dispatch,
  atomically finalize the same strict receipt, and never retain or render raw
  provider data. Reservation failure makes zero requests; a post-call receipt
  write fault is receipt_failure without an inferred model verdict. A
  mismatched binding rejects before dispatch and cannot consume or reuse retry
  budget.
- Attempt 2 is available only after explicit rate_limit, upstream, transient,
  network, deadline, runner_failure, or receipt_failure at attempt 1. Valid
  PASS/FAIL/BLOCKED and every 400/401/402, unknown, parse, schema, identity,
  malformed-tool, opaque, or untrusted outcome is terminal; attempt 2 never
  permits a third dispatch.
- This is a material scope change. The prior Captain review is superseded,
  S22 moves only to design_review, and a fresh Captain PROCEED plus
  acknowledgement is required before implementation. S20 remains blocked and
  untouched until a fresh S22 verifier PASS. No provider/model call occurred
  in this Planner replan.
- Reconciliation against the authoritative T1 worktree confirms that the
  immediately preceding serial slice, S21-openai-structured-envelope, is
  verified/PASS at immutable start ed0badf68673f0af84834458f07be0792555484f.
  The release-wt copy is corrected to that verified state; S22 must remain
  gated if this upstream evidence is not preserved.

## 2026-07-18T08:38:47+10:00 - Planner narrow safety replan

- The Coach approved a narrow S22-only correction after pre-live audit found
  that the registered `sworn.llm_check` MCP tool can expose a
  provider-response-derived error message even when receipts and generic JSON
  are sanitized. S22 now owns the MCP adapter/test touchpoints and C-17:
  provider/model errors preserve MCP error/non-success semantics but expose
  exactly `llm_check: provider request failed`, with deterministic reachability
  and leak-canary evidence required.
- The receipt contract now explicitly covers the second-fault case: if a
  post-rename finalization error is followed by failed reservation restoration,
  a final model verdict must never remain trusted. Only a durable
  `receipt_failure`/`UNPARSEABLE` record with unavailable exit semantics may be
  retained or surfaced, and the path fails closed.
- S22 preflight must mechanically bind its declared S21 slice/release identity,
  authoritative status path/commit, immutable start, verified/PASS result,
  non-empty verdict time, and `verifier_was_fresh_context: true`; every absent
  or mismatched fact makes zero provider requests.
- The release assembly first merged `release/v0.2.0` cleanly at `8d58384`.
  T1 has preserved uncommitted S22 proof/spec work, so this planning change is
  deliberately not forward-merged into that dirty worktree. A fresh
  Implementer must self-heal from the committed assembly branch and retain the
  preserved work without reset, checkout, stash, or provider dispatch.
- No provider/model call, credential inspection, source-code edit, proof
  completion claim, S20 action, model-default change, routing change, proxy
  expansion, fallback, or retry-policy broadening occurred. The prior Captain
  review remains superseded; fresh Captain PROCEED and acknowledgement are
  required before implementation resumes.
