# Journal — S21-openai-structured-envelope

## 2026-07-17T07:57:25+10:00 — Planner replan

- Added as a planned T1 prerequisite immediately after verified
  S04-typed-reference-ambiguity and immediately before blocked
  S20-v015-parity-portable-fixture.
- Trigger: at T1 head
  69238f0b011b7e2965ede64231e17ba373a510dd, the configured OpenAI
  structured-output request rejects the exact canonical
  llm-check-report-v1 before model emission because that schema contains
  top-level allOf conditionals. No accepted emitted check exists at this
  boundary.
- S04 remains verified and immutable. Its exact vendored prompt/schema bytes,
  local canonical validation, and requested/emitted generic check equality
  remain the authority. The new compiler is only an OpenAI wire envelope below
  that authority.
- The only recognised generic source identity is canonical
  https://baton.sawy3r.net/schemas/llm-check-report-v1.json with SHA-256
  ed38b77823af1b329c1dc7d8427b08849f15690d5afa9625e196505bdfa5b65b.
  The deterministic envelope is named
  llm-check-report-v1-openai-envelope. Unknown/digest-mismatched generic
  identities and spec-ambiguity-report-v1 reject locally before HTTP.
- This is a non-Type-1 technical correction ratified under the Coach's
  standing orchestration authority. No product code, main, real homes, S04
  source/lifecycle, S20 source/lifecycle, or S20 preserved evidence is changed
  by this planner session.
- S20 retains immutable start
  08dd38f81e466d3288ff4bf64953cfc90ea6063c, semantic commits
  edad0fa8a75ab3b4a1938bdaf856c7973be72107 and
  f3da6a49c3f89f0883e265befd30d1eb099d6a90, resume
  bef712dbc629678d7bf2579d3beb560e2b025c0a, and its blocked evidence.
  It may resume only after a fresh S21 verifier PASS, then must rerun its own
  readiness and maintainability evidence and perform the credentialed OpenAI
  exact-base smoke that yields accepted check: ac-satisfaction.

## Handoff

- Stop at planned. Do not create a design TL;DR or implement S21 in this
  planner session.
- The next action is a fresh S21 Implementer session on T1-foundation. It must
  begin from the propagated track branch, use deterministic fake endpoints for
  both OpenAI paths, and leave S20 untouched.

## 2026-07-17T09:31:00+10:00 — Implementation in progress

- Recorded the implementation anchor `ed0badf68673f0af84834458f07be0792555484f`.
  It is the exact `start_commit` for the S21 verifier diff; all product work is
  confined to the planned model, gate-test, and CLI-test surfaces after it.
- Added a closed-world source compiler selected only by the canonical generic
  report `$id` and pinned SHA-256 digest. It emits one sealed
  `llm-check-report-v1-openai-envelope` for explicit OpenAI Responses and
  chat/completions profiles, while mismatched/future generic identities and the
  dedicated ambiguity map reject before HTTP.
- Direct and proxy construction now carry the same explicit profile and wire
  mode. xAI, forced tool calls, and unprofiled OAI-compatible values retain
  their supplied-schema paths. `internal/gate/llmcheck.go`, canonical protocol
  bytes, S04, and S20 are not modified.
- Focused compiler/wire/gate/built-binary fakes and the full repository suite,
  vet, and build pass with `GOFLAGS=-buildvcs=false`. The shared worktree's
  normal Go VCS stamp lookup fails before compilation; a clean independent
  clone will provide the normal stamped validation after this durable commit.

## 2026-07-17T08:13:39+10:00 — Implementer design checkpoint

- Read C-02, C-14, the S21 contract, verified S04 artefacts, blocked S20
  lifecycle records, and the current model, gate, and public CLI seams from
  the T1 track worktree. S21 is ready for Captain review; no product code or
  evidence bundle was produced.
- The existing generic strict projection preserves canonical `allOf` branches,
  which explains the provider's pre-emission rejection. The proposed fix is a
  closed-world OpenAI transport compiler selected only by the canonical
  generic-report `$id` plus pinned source digest. It emits one fixed envelope
  below, never instead of, S04's canonical semantic validation and requested/
  emitted-check equality.
- Provider identity will be explicit at direct and proxy construction. Native
  xAI response-format, forced tool-call, and unprofiled OAI-compatible paths
  retain their supplied-schema behavior; endpoint URL or Go concrete type is
  not authority for the envelope.
- The public Responses binary path needs the documented `openai/` base-URL
  override applied to `OpenAIResponses`, in addition to the existing
  `openai-completions/` override, so deterministic fake endpoints can prove
  both wire formats without credentials or model spend.
- S20 remains unchanged and blocked. Its credentialed exact-base smoke is
  explicitly later evidence only after a fresh S21 verifier PASS, followed by
  S20's own readiness and maintainability reruns.

## Handoff

- `design.md` now records the closed-world compiler, explicit profile, error,
  semantic-authority, lifecycle, and reachability design. Run a fresh Captain
  `/design-review S21-openai-structured-envelope 2026-07-15-baton-v0.15-conformance`
  before any implementation.

## 2026-07-17T08:29:21+10:00 — Automatic Coach acknowledgement and Captain PROCEED

- Under the Coach's standing instruction to orchestrate this release, the
  Captain's `PROCEED` verdict in `review.md` (commit `0bccd6e`) is
  acknowledged. There are no `[escalate]` pins and no new Type-1 decision to
  seek.
- Apply pin 1 inline: make the generic-report-family matcher a closed-world
  canonical `$id` plus exact-digest decision, never a broad `$id` heuristic.
- Apply pin 2 inline: propagate explicit provider profile and wire mode through
  both direct and proxy construction, including the deliberate deprecated
  Responses alias behavior.
- Apply pin 3 inline: make built-binary local-rejection tests hermetic, assert
  exit `2`, and prove zero HTTP dispatch on every rejected schema path.
- Apply pin 4 inline: preserve `spec-ambiguity-report-v1` as the dedicated map
  contract; this slice rejects it locally for OpenAI response-format rather
  than flattening or reconstructing it.
- Proceed to `in_progress` only in a fresh Implementer session. That session
  must stop at `implemented`; only a fresh S21 verifier PASS can permit S20 to
  resume its own readiness, maintainability, and credentialed exact-base smoke.
