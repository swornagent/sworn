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

## 2026-07-18T13:26:43+10:00 — Implementer restores the design-review gate

- The track worktree is clean at the propagated replan commit, the current
  status validates against `slice-status-v1`, the immutable start resolves and
  is an ancestor of the track head, and the empty pending cycle-0
  maintainability record has no exhausted committed history.
- The existing `review.md` predates the narrow safety replan and is explicitly
  superseded by the later Planner journal entry. Its design also said MCP
  semantics were unchanged, which no longer covers AC-11, the AC-06
  post-rename/restoration double fault, or AC-12's mechanical S21 evidence gate.
- Refreshed `design.md` to trace all twelve ACs and returned the lifecycle to
  `design_review`. No source, test, provider, model, credential, proof receipt,
  or S20 action occurred. A fresh Captain PROCEED and Coach acknowledgement are
  required before implementation resumes.

## 2026-07-18T14:24:21+10:00 — Fresh Captain PROCEED acknowledged; implementation resumes

- Captain commit `798e114c` reviews design commit `19d2ab1b`, records
  `DECISION: PROCEED`, and has no escalate pins. Brad acknowledged the verdict
  and authorized the Implementer to proceed with all six apply-inline pins.
- Pin 1: add a fail-closed trust guard and deterministic double-fault evidence
  so an unacknowledged renamed verdict is never accepted.
- Pin 2: added `internal/mcp/lint.go` and `internal/mcp/lint_test.go` to
  `planned_files`; their scope is limited to the stable provider-error
  diagnostic plus registered-tool reachability and leak canaries.
- Pin 3: re-anchor the active runtime constant, receipt binding, lookup, and
  tests to v0.16 while retaining v0.15 only as immutable historical provenance.
- Pin 4: record a cohesion audit for the receipt state-machine, persistence,
  rendering, and runner seams before completion.
- Pins 5-6: preserve the narrow typed retry classifier independently of legacy
  `IsTransient`, and keep `openrouter/z-ai/glm-5.2` proof-only with no model
  default, catalogue, or routing-policy change.
- The `beast` effort/complexity rating remains accurate against the live code
  and is now Implementer-confirmed. The immutable start and empty pending
  cycle-0 maintainability ledger are preserved byte-for-byte.

## 2026-07-18T14:40:15+10:00 — Receipt cohesion audit and deterministic implementation checkpoint

- Audited `internal/gate/llmcheck_receipt.go` across its state-machine,
  persistence, rendering, and runner seams. The module remains one cohesive
  fail-closed aggregate around the private `ProofReceipt` invariant: the runner
  can return only a sanitized outcome, persistence owns reservation/finalization
  and the durable trust guard, and rendering accepts only the strict metadata
  record. Splitting these private responsibilities would require an additional
  cross-package or exported intermediate contract that could represent an
  unguarded final verdict, weakening the atomic invariant rather than creating
  an independently useful seam. The existing seams remain independently
  exercised through injected runners/writers and public rendering tests, so no
  module split is warranted for S22.
- The post-rename plus failed-restoration double fault now leaves a durable
  metadata-only trust guard; later readers reject the renamed model verdict and
  the caller sees only `receipt_failure` / `UNPARSEABLE` / unavailable exit
  semantics. The exact regression passed after one synthetic dispatch.
- The MCP adapter retains error/non-success semantics while returning exactly
  `llm_check: provider request failed`; registered-tool reachability and leak
  canaries passed. The two MCP files remain the only added planned touchpoints.
- Active runtime binding, S21 receipt/status lookup, and tests are anchored to
  v0.16. The retry classifier remains the narrow typed proof-only boundary, and
  `openrouter/z-ai/glm-5.2` remains proof-only with no default, catalogue, or
  routing-policy change.
- Deterministic gates passed: targeted S22 tests, `go test ./...`, `go vet
  ./...`, `make build`, and the two built-command reachability tests. No live
  provider/model dispatch or credential inspection occurred.

## 2026-07-18T14:49:34+10:00 — Attempt 2 is terminal; fail-closed Planner handoff

- Proof commit `ba6648a9` durably captured every AC-12 precondition before
  dispatch: exact fresh S21 evidence, acknowledged Captain PROCEED, targeted and
  full tests, vet, build, built-command reachability, and the current binary's
  zero-cost deterministic proof-bundle PASS.
- The first command omitted the required `SWORN_DIRECT=1` process flag and was
  rejected during deterministic preflight. It created no reservation, made no
  provider request, and consumed no retry budget. The corrected invocation was
  the sole attempt-2 dispatch.
- Native attempt 2 finalized the strict metadata-only receipt with class
  `opaque`, result `UNPARSEABLE`, and process exit code 2. No provider payload,
  endpoint, header, request/response body, finding, prompt, diff, credential, or
  key was retained or rendered.
- AC-09/AC-10 make every attempt-2 non-final outcome terminal. The two-attempt
  budget is exhausted: no retry, fallback, provider/model/transport switch,
  third dispatch, completion claim, maintainability cycle, verifier dispatch,
  or S20 activity is authorised.
- S22 moves to `blocked` with a machine-readable AC-12 violation and is handed
  to the Planner for explicit re-scope or closure. The deterministic
  implementation remains committed and all sanitized receipt evidence is
  preserved.

## 2026-07-18T15:15:40+10:00 — Planner reconciliation and configured-values recovery ratification

- The pre-sync and post-sync board-oracle projection integrity gates passed.
  `release-wt/2026-07-15-baton-v0.16-conformance` is already current with
  `release/v0.2.0` at Planner start `0611d778a972aace9a3bb0e5e064a876245e45ed`.
  T1 is the only in-progress track; T2, T5, T6, and T7 remain planned with no
  materialised track refs. All eleven T1 specs have zero release-vs-track drift.
- Seeded every started T1 lifecycle record from authoritative owner ref
  `track/2026-07-15-baton-v0.16-conformance/T1-foundation`. The unchanged
  status blob ids are S01 `db5ecd03c0488e510e0289dcd0335499a7e5fb78`,
  S02 `bc335a082ca08e1b02333901b2f51d5612b7c570`, S19
  `46ebadb92c46d1767f42b3e4d48f377ab49fff88`, S04
  `b22b0d7c4b4c5c0853e014fa1988411485fdb3a7`, S21
  `4788a66e5b4329e4c21d604236065d64c71b3ed4`, and S20
  `0824242421c09f456197117ad062c808ca1c25c3`. S22 was seeded exactly from
  owner blob `a0964d15580925345ff2e8aed316e6adfb4ff0ec`; its pending cycle-0
  `maintainability` object is preserved byte-for-byte.
- Diagnosed trigger: the sole authorised GLM-bound attempt 2 finalized as
  `opaque` / `UNPARSEABLE` / exit 2, so the former contract correctly blocked
  all further dispatch. S20 remains independently blocked and untouched behind
  the fresh-S22-PASS gate.
- Brad ratified a factual S22 contract correction: preserve attempts 1 and 2
  byte-for-byte and permit exactly one separately governed attempt 3 using the
  verifier model resolved from the current standard config, with no CLI model
  override. The strict receipt records the resolved model ID only; config path,
  endpoint, credentials, and payload remain excluded.
- Attempt 3 is not a broad retry-classifier extension. Its authority is the
  explicit Planner/Coach amendment plus fresh Captain review and deterministic
  proof gates. Unsupported or unconfigured values reject before dispatch;
  every attempt-3 outcome is terminal; no fallback, fourth dispatch, or S20
  activity is authorised before a fresh S22 verifier PASS.

## 2026-07-18T15:26:22+10:00 — Configured-recovery-v2 contract emitted

- Corrected S22 in place because the inbound BLOCKED diagnosis identified a
  contract-bound proof-model dead end, Brad explicitly ratified the correction,
  S22 is unmerged, and the existing implementation does not yet satisfy it.
  `verification.result` is cleared to `pending`; lifecycle becomes
  `failed_verification` for a fresh Implementer design/implementation cycle.
- Historical attempts 1 and 2 remain immutable v1 receipts. Added a separate
  `llm-check-proof-receipt-v2` planning schema with the identical metadata
  allowlist, `record_version: 2`, and `attempt: 3` constant. This prevents the
  replan from silently changing the schema beneath historical evidence.
- C-16 and AC-06/09/10/12 now distinguish unchanged typed retry classification
  from the one administrative configured recovery. Attempt 3 rejects a model
  flag, resolves only `verifier.model` from standard config, persists only the
  resolved model ID, and is terminal. Attempt 4 and every fallback remain
  prohibited.
- The release topology is unchanged: 24 slices, five tracks, no dependency or
  shared-touchpoint change. The rendered matrix adds only the S22 v2 schema
  path under T1.
- Deterministic planning gates passed: `sworn lint ac` (149 ACs), `sworn lint
  trace` (16 needs / 149 ACs), `sworn reqvalidate` (24/24), `sworn specquality`
  (24/24), and `sworn designfit` (24/24). All edited JSON parses and the
  rendered `index.md` was regenerated from the unchanged board plus revised
  spec/status.
- No model/provider call ran in this Planner session. Why: the ratified safety
  boundary reserves configured model dispatch for the native attempt-3 receipt
  only after revised implementation, deterministic proof, and fresh Captain
  review. Tracking: S22 AC-12 and the next `/implement-slice` design cycle.
  Acknowledgement: Brad explicitly invoked this replan after requesting the
  configured-values recovery and accepted the fresh-review boundary.

## 2026-07-18T20:32:22+10:00 — R-05 classifier/recovery contradiction corrected

- Reconciled the post-base-sync board projection from live refs: S22 is
  `failed_verification` with `verification.result: pending` on in-progress
  T1; S20 remains independently `blocked`; the other four tracks remain
  planned. All T1 specs have zero release-vs-track drift.
- Seeded and validated every started T1 lifecycle record from authoritative
  owner ref
  `track/2026-07-15-baton-v0.16-conformance/T1-foundation`. Exact status blobs:
  S01 `db5ecd03c0488e510e0289dcd0335499a7e5fb78`, S02
  `bc335a082ca08e1b02333901b2f51d5612b7c570`, S19
  `46ebadb92c46d1767f42b3e4d48f377ab49fff88`, S04
  `b22b0d7c4b4c5c0853e014fa1988411485fdb3a7`, S21
  `4788a66e5b4329e4c21d604236065d64c71b3ed4`, S22
  `5bca502d3aa28bddf79eddcf68eec1d6242fb91b`, and S20
  `0824242421c09f456197117ad062c808ca1c25c3`. Each release copy already
  matched its owner blob, so no lifecycle field or maintainability object was
  changed.
- Diagnosed the new planner blocker: R-05 still prohibited any third call even
  though A-23 and AC-06/09/10/12 authorize exactly one separately governed
  configured-recovery-v2 attempt 3. That wording made the live spec internally
  contradictory and prevented a valid fresh design.
- Corrected only R-05's mitigation. Attempt 2 remains terminal and
  non-retryable under the unchanged classifier; the administrative gate may
  authorize exactly terminal attempt 3 after the AC-09/AC-12 prerequisites
  without reclassifying attempt 2. Attempt 4 and fallback remain prohibited.
- Release topology, touchpoints, dependencies, status, historical receipts,
  implementation, and provider/model policy are unchanged. Fresh Implementer
  design and Captain review remain required before source or provider action.

## 2026-07-18T20:38:19+10:00 — Configured-recovery design restored

- Confirmed the Planner's R-05 correction is live on both the track and release
  refs: attempt 2 remains terminal for the unchanged typed classifier, while
  the separately ratified administrative gate may authorize only terminal
  attempt 3 after AC-09/AC-12 prerequisites.
- Replaced the stale attempt-2 design with a numbered §1–§6 Design TL;DR that
  carries all three status decisions, the exact config-only v2 attempt-3 file
  plan, preservation boundaries, acceptance evidence, and explicit no-default,
  no-fallback, no-attempt-4 policy.
- `d02899f6` remains an unapproved implementation candidate already present in
  the live tree. No source, test, proof, provider/model, credential, receipt, or
  S20 action occurred in this session. The candidate must be assessed only
  after a fresh Captain PROCEED and Coach acknowledgement.
- Returned S22 to `design_review` with its immutable start, pending cycle-0
  maintainability ledger, pending verification record, historical receipts,
  and configured-recovery authority preserved.

## 2026-07-18T22:34:24+10:00 — Captain PROCEED acknowledged; implementation resumes

- Captain commit `2cab8cd8` records `DECISION: PROCEED`, `CONSTITUTIONAL: yes`,
  five mechanical pins, two memory-cited pins, and no escalation. Brad supplied
  the acknowledgement and directed all seven pins to be applied inline.
- Critical implementation gates are: separate immutable GLM attempt-1/2
  identity from the config-selected attempt-3 identity; make the durable
  Captain/lifecycle/proof prerequisites authoritative before reservation; and
  reject orphan `--configured-recovery` before model setup.
- Additional gates cross-validate attempt-3 records against the Planner-owned
  v2 schema, retain exact AC-05/07/08 tests, keep typed provider errors separate
  from administrative authority, and use
  `[[capability-based-model-selection-ratified]]` for config-only selection.
- Transitioned to `in_progress` with immutable start
  `a09b0e46df465862d00469d4aef2a997442b3d5b`, empty pending cycle-0
  maintainability, pending verification, and historical receipts unchanged.

## 2026-07-18T22:49:18+10:00 — Captain pins closed deterministically

- Split configured recovery into an immutable GLM history binding and a
  separately config-resolved attempt-3 binding. Attempts 1 and 2 now require
  the exact `receipt_failure/UNPARSEABLE/unavailable` and
  `opaque/UNPARSEABLE/2` tuples; per-field mutations dispatch zero calls.
- Made `in_progress`, durable Captain acknowledgement and PROCEED verdict, a
  schema-valid current proof bundle, and passing targeted/full-suite/vet/build
  evidence mandatory before attempt-3 reservation. Orphan
  `--configured-recovery` now exits before model setup.
- Cross-asserted rendered and decoded v2 receipts against the Planner-owned
  schema's strict fields, constants, enums, and additional-property ban. The
  administrative gate remains separate from the unchanged typed retry
  classifier and does not consult raw errors, `IsTransient`, or unknown
  outcomes for authority.
- Proved a configured attempt-3 model different from historical GLM is the
  exact request/receipt model even when `SWORN_VERIFIER_MODEL` names another
  value. Config remains read-only, capability-gated, and fallback-free.
- Targeted S22 tests, the full Go suite, `go vet ./...`, `make build`, and the
  exact AC-05/07/08 endpoint-isolation and malformed-tool preservation tests
  all passed. No provider request or attempt-3 receipt has occurred yet.
