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

## 2026-07-17T18:27:06+10:00 — Implementer recovery guards and deterministic proof preflight

- Added only the two ratified direct-OpenRouter transport guards. `FunctionCall`
  now retains whether the wire value for `function.arguments` was literal JSON
  `null`, so direct forced-tool extraction can reject it rather than silently
  treating it as an empty Go string. The same direct-only extraction now rejects
  a returned tool call whose `type` is not `function` before canonical report
  acceptance. Existing legacy forced-tool behavior remains unchanged.
- `TestOpenRouterStructuredRejectsInvalidToolCall` proves both cases make
  exactly one provider request and fail locally with no repair, fallback, or
  second request. The focused model and built-command/gate suites, full
  `go test ./...`, `go vet ./...`, and `make build` all passed. The full
  deterministic gates ran in an isolated no-credential, no-model environment;
  `GOFLAGS=-buildvcs=false` only avoids this host's unrelated VCS-status probe.
- A local `go test -cover ./internal/model -count=1` measurement passed at
  81.4% statements. The role-prompt `sworn coverage` command is absent from
  the current binary (`unknown command`). This is not an S22 blocker: why — a
  coverage command is unrelated scope; tracking — `sworn#122`; acknowledgement
  — the Coach explicitly accepted the artifact-specific AC-to-test matrix plus
  local Go coverage measurement in its place. No command or source was added
  for coverage.
- Created a valid pre-live proof bundle with the AC-to-evidence mapping and the
  two acknowledged Rule-2 entries above. It deliberately does not claim AC-06
  delivered. The current binary's `verify` wrapper requires a verifier model
  even for its documented deterministic first-pass, so the isolated preflight
  used only a synthetic direct OpenRouter construction with an unroutable local
  endpoint. It returned `PASS` at zero cost; the first-pass does not dispatch
  its constructed verifier. No real credential, outbound request, or AC-06
  provider command has run in this recovery session.

## 2026-07-17T18:32:31+10:00 — AC-06 fail-closed handoff

- Check identity: `spec-ambiguity`
- Model ID: `openrouter/z-ai/glm-5.2`
- Immutable start commit: `a09b0e46df465862d00469d4aef2a997442b3d5b`
- Process exit code: `unavailable`
- Result: `UNPARSEABLE`

The raw temporary file was destroyed. The exactly-one AC-06 budget is consumed:
there is no retry, fallback, repair, `implemented` or `verified` transition, or
S20 activity. S22 is blocked and returned to the Planner for a new decision;
this is not a fresh verifier verdict.

## 2026-07-17T18:44:24+10:00 — Fail-closed handoff state recorded

- `status.json` now records `state: blocked` and `verification.result:
  blocked`. Its machine-readable AC-06 violation records the exact-one
  `UNPARSEABLE` result and the absence of a safely available process exit code.
- Recovery is explicitly routed to new human authority and Planner ratification
  before any further provider action. The consumed proof budget, no-retry/no-
  fallback constraint, and S20 hold remain unchanged.
- The synthetic-only deterministic proof-gate PASS remains evidence of the
  current binary's no-model-dispatch first pass. It is not AC-06 evidence and
  is not a reason to alter this blocked handoff.

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

## 2026-07-18T07:03:29+10:00 — Automatic Coach acknowledgement of revised Captain PROCEED

- Under the Coach's standing instruction to orchestrate this release, the fresh
  Captain review in commit `223f687` is acknowledged. Its `PROCEED` verdict
  supersedes the earlier review and has no `[escalate]` pins or new Type-1
  decision.
- Apply both inline pins during implementation: keep
  `openrouter/z-ai/glm-5.2` proof-only (never a default, catalogue, or
  routing change), and retain the AC-05 direct-base and proxy-isolation
  regressions in the proof evidence.
- This acknowledgement authorizes only the normal S22 implementation lifecycle.
  It does not authorize a provider call until the native receipt facility,
  deterministic gates, proof bundle, and all S22 preconditions are complete;
  the one remaining live attempt remains attempt 2 only, with no fallback or
  third dispatch.
