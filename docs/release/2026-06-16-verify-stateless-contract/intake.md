---
title: Release intake — 2026-06-16-verify-stateless-contract
description: Patch on v0.1 — give `sworn verify` its own stateless prompt/parser contract so the verification gate stops emitting spurious BLOCKED. Fixes the verify gate shared by `sworn verify` and `sworn run`.
---

# Release Intake: `2026-06-16-verify-stateless-contract`

## Release goal

Make the verification gate **actually return verdicts**. Today `sworn verify`
dispatches the **agentic Baton verifier role prompt** (`verifier.md`) into a
**stateless, tool-free, single-shot** chat completion, then rejects any reply
whose first token is not a verdict. The model is told it is an investigating
agent — so it reaches for tools it does not have (`<tool_call …>`) or opens with
markdown preamble — and the strict parser converts those shapes into
`BLOCKED / unparseable_verdict`. Across three model families the parse rate is
~1-in-3 on identical inputs.

This patch gives the stateless `verify` path its **own** prompt/runtime contract:
a stateless judge prompt (not the agentic role prompt) plus a tolerant-but-still-
fail-closed parser. The deterministic plumbing is already correct and stays
untouched — the defect is the prompt handed to the path, not the wire request.

"Shipped" = on a known-good spec+diff, `sworn verify` returns a parseable
`PASS`/`FAIL`/`BLOCKED`/`INCONCLUSIVE` (no `unparseable_verdict` from format
variance) across ≥3 providers, and `sworn run`'s verify gate lands a parseable
verdict end-to-end.

## Source of truth

- **Stakeholder**: repo owner.
- **Origin**: live dogfood, 2026-06-16. Binary rebuilt from `release/v0.1.0`.
  Symptom, verified root cause (`file:line`), and evidence captured in the
  internal findings note (private, not in this public repo).
- **Builds on**: `2026-06-15-e2e-turnkey-loop` (this defect lives in the verify
  gate that release shipped — the strict parser landed in `S01-verifier-core`,
  and the agentic prompt was wired onto the stateless path in
  `S04-embed-baton-prompts`).

## What the gate must do (the fix)

1. The stateless `verify` path uses a **stateless judge prompt** that states
   plainly: only the SPEC and DIFF are available — no tools, no repo, no test
   execution — and the reply MUST begin with exactly one of
   `PASS` / `FAIL: …` / `BLOCKED: …` / `INCONCLUSIVE: …` as the very first
   characters, no preamble, no markdown, no tool calls. It keeps the
   four-verdict semantics (and the BLOCKED-vs-INCONCLUSIVE distinction) of the
   Baton verifier role.
2. A **tolerant-but-safe parser** scans the first non-empty line for a leading
   verdict token after stripping markdown emphasis/fences, instead of requiring
   the literal first byte. Still fails closed on genuine ambiguity.
3. The headline turnkey journey — `sworn run` — gets the fix for free (its verify
   gate calls the same `verify.Run`); this release proves that end-to-end rather
   than assuming it.

## Root-cause scope correction (vs the original findings note)

Two scope claims in the findings note were **disproven** while planning this
release and are corrected here:

- **`verifier.md` has exactly one consumer — the stateless `verify.Run` path
  (`internal/verify/verify.go:21`).** There is no agentic verifier loop in the
  binary, so "verifier.md stays the role prompt for the run agent loop" is not
  accurate. `sworn run`'s *implementer* step uses `prompt.Implementer()` + the
  tool loop; its *verifier* step uses the stateless `verify.Run`.
- **`sworn run` IS affected.** `internal/run/run.go:232` calls `verify.Run`, the
  same broken stateless path — so the run loop's verify gate hits the identical
  defect and the loop cannot reach `verified` → gated-merge. The run loop's
  *implement* step is unaffected.

## Constraints and non-negotiables

- **Native Go, single binary, zero runtime deps.** The stateless prompt is a new
  `go:embed` text file; the parser is stdlib `strings`. No SDK, no provider lib.
- **Fail-closed throughout.** A tolerant parser must never widen the door to a
  false `PASS`; ambiguity still resolves to `BLOCKED`/`INCONCLUSIVE`.
- **Provider-neutral baseline.** The fix must hold across the OpenAI-compatible
  surface (deepseek / groq / gemini-compat). Structured output
  (`response_format: json_schema`) is **opt-in per provider, not the baseline** —
  support is uneven and forcing it trades away provider neutrality.
- **Public-safe.** This release's specs and any committed test fixtures are
  technical only. Regression fixtures MUST be **synthetic** spec+diff — the
  private dogfood slice spec/diff used as evidence must never be committed here.
- **`verifier.md` stays vendored.** It remains embedded as the canonical Baton
  protocol artefact surfaced by `sworn version` / provenance, even after it is no
  longer the system prompt on the verify path (see decision below).

## Adjacent / out of scope (Rule 2 deferrals)

- **Structured-output verdict (tool-call / `response_format` JSON schema).**
  Deferred. Why: uneven provider support; trades provider neutrality for
  determinism. Tracking: roadmap (dovetails with the model-conformance-probe
  idea). Acknowledged: 2026-06-16. The stateless prompt + tolerant parser is the
  provider-neutral fix and is sufficient.
- **An agentic, tool-using verifier** (a real consumer of `verifier.md`, parallel
  to the implementer loop). Deferred to a later release; not required to fix the
  gate.
- **Live multi-provider conformance as a committed test.** The ≥3-provider
  acceptance is a manual/dogfood reachability step (it needs network + keys), not
  a CI unit test. The committed regression test uses synthetic fixtures.

## Decisions made during planning

### 2026-06-16 — Stateless verify gets its own prompt; verifier.md is not mutated

- **Context**: `verifier.md` is the agentic role prompt vendored verbatim from the
  open Baton protocol; the stateless path needs a different contract.
- **Decision**: add a **new** sworn-authored embedded prompt for the stateless
  path (e.g. `internal/prompt/verify-stateless.md` + a `prompt.VerifyStateless()`
  accessor). Do **not** edit `verifier.md` — it stays the vendored protocol
  artefact. After the switch, `verifier.md` is embedded-but-unreferenced-by-code;
  that is accepted on purpose (provenance / `sworn version` surface / future
  agentic verifier). Dropping it from the embed set is explicitly **not** done in
  this release.

### 2026-06-16 — Single track, sequential

- **Context**: the two core slices both edit `internal/verify/verify.go`, so they
  cannot be touchpoint-disjoint parallel tracks.
- **Decision**: one track `T1-verify-contract`, slices run sequentially
  S01 → S02 → S03.

## Proposed slice decomposition

- `S01-stateless-verify-prompt` — new stateless judge prompt + accessor; switch
  the verify path's system prompt off `verifier.md`.
- `S02-tolerant-verdict-parser` — first-non-empty-line, markdown-tolerant,
  still-fail-closed parser + synthetic-fixture regression test.
- `S03-run-loop-verify-reachability` — end-to-end proof that `sworn run`'s verify
  gate lands a parseable verdict (the Rule-1 reachability slice through the
  user-facing command, not the leaf package).

## Open questions

- None outstanding.

### 2026-06-16 — Version / integration branch: land on `release/v0.1.0`

- **Context**: no git tags exist and nothing has been released yet.
- **Decision** (owner, 2026-06-16): fold this fix into `release/v0.1.0` before
  first ship — this is a pre-release bug fix, not a post-release `v0.1.1` patch.
  All slices' `release_base` = `release/v0.1.0`.
