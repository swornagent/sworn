---
title: 'Proof bundle — ADR-0011 keystone, step 3 (verifier-verdict-v1 pilot)'
description: 'The Rule-7 agentic verifier emits a schema-constrained verifier-verdict-v1 object via ChatStructured, validated fail-closed; the prose HasPrefix verdict scrape and the extractViolations prose-split are deleted. Supersedes #32/#34.'
date: 2026-06-30
---

# Proof bundle — ADR-0011 keystone, step 3

## Scope
Close the one live ADR-0009 invariant breach in the hot path: the agentic
Rule-7 verifier (`internal/verify.RunAgentic`) now **emits** its verdict as a
schema-constrained `verifier-verdict-v1` structured-output object (via Step 2's
`ChatStructured`) and validates it with `baton.ValidateSchema` (Step 1), instead
of replying in prose that a `HasPrefix(upper,"PASS")` scrape parsed. The
prose-splitting `extractViolations` is deleted; violations come off the typed
record. Any non-emittable / malformed / schema-invalid verdict fails closed to
INCONCLUSIVE. The dead stateless prose classifier (`orchestrator.Interpret`) is
removed. Supersedes #32 and #34. Anchored to #22.

## Continuation-handshake divergence (surfaced before implementation)
The step-2 handoff pointed the pilot at `orchestrator.Interpret` /
`parseInterpretResult`. Live wiring showed that path is **dead code** (defined +
unit-tested, never called in production; only its `InterpreterInconclusiveSentinel`
const is consumed by `scheduler/worker.go`). The actual live breach is
`verify.RunAgentic` → `parseVerdict`, wired at `slice.go:583` and
`cmd/sworn/verify.go:92`. The pilot was retargeted accordingly (Brad acknowledged,
2026-06-30).

## Design decisions (Rule 9)
- **Pilot target (retarget):** `verify.RunAgentic`, the live agentic verifier — not
  the dead interpreter. **Type-2** (reversible). Acknowledged.
- **Delete dead interpreter now (Brad, decided):** removed `Interpret` /
  `parseInterpretResult` / `firstInterpretLine` / `interpreterSystemPrompt` + the
  dead `captureVerifier` (`slice.go`). KEPT the live `InterpreterInconclusiveSentinel`
  + `ErrInterpretInconclusive` contract that `worker.go` consumes. **Type-2.**
- **INCONCLUSIVE = Option A / defer (Brad, decided):** the verifier emits a typed
  verdict that can be INCONCLUSIVE; the merge gate stays fail-closed exactly as
  today (triage folds INCONCLUSIVE into the FAIL bucket → `failed_verification`).
  The slice-status leaf `result` enum is NOT touched. The deferred D4 leaf-enum
  addition is tracked as **#37** (Rule-2 deferral: why+tracking+ack). **Type-1**
  classified; human-decided.
- **Structured plumbing (Type-2):** no new interface — `verify.RunAgentic`
  type-asserts the existing `model.StructuredOutput` (Step 2) on the verifier
  `agent.Agent` value (the concrete agent IS the model client per
  `run.newAgentFromModel`). A non-structured driver fails closed to INCONCLUSIVE.
- **Two schema views (ADR §3.3 b/g):** the model emits a tight judgement-only
  subset (no identity/telemetry, no `minLength`/`format` — strict-mode safe); the
  binary stamps `schema_version`/`$schema`, then validates the stamped object
  against the canonical `verifier-verdict-v1.json`. Canonical `required` is the
  model-authored core (`schema_version`, `verdict`, `rationale`); identity and
  telemetry are validated-if-present. **Divergence from the literal §3.3 sketch**
  (which `required` slice_id/release) is forced by §3.3(g) "model payload is
  judgement-only" — a required identity triple would make every emission invalid.

## Files changed
`git status --short` (working tree; not yet committed at bundle time):
```
A  docs/captures/2026-06-30-keystone-step2-verify.md   (step-2 Rule-7 verdict, prior)
A  internal/baton/schemas/verifier-verdict-v1.json     canonical schema
M  internal/baton/schemas/embed.go                     embed var + SchemaMap entry
M  internal/baton/validate_schema_test.go              TestValidateSchema_VerifierVerdict
M  internal/verdict/verdict.go                          Result.Violations []string + Routing
M  internal/verify/verify.go                            RunAgentic→ChatStructured; acceptStructuredVerdict; verifierEmitSchema; delete parseVerdict/firstVerdictLine/stripMarkdown
M  internal/verify/verify_agentic_test.go               structured emission tests + fail-closed cases
M  internal/verify/verify_test.go                       remove TestParseVerdict* (deleted scrape)
M  internal/orchestrator/interpreter.go                 delete dead classifier; keep sentinel/Err contract
M  internal/orchestrator/interpreter_test.go            remove dead-classifier tests; keep Err tests
M  internal/run/slice.go                                extractViolations→lastVerdict.Violations + Routing; delete extractViolations + captureVerifier
M  internal/run/run_test.go                             verifierAwareAgent.ChatStructured + structuredVerdictReply bridge; update 2 obsolete-prose tests
M  internal/run/slice_test.go                           passingVerifierAgent.ChatStructured
```

## Test results
Slice-relevant commands (full suite is the merge gate's job, but it was also run — see below):
- `go build ./...` → exit 0
- `go vet ./internal/verify/ ./internal/baton/... ./internal/orchestrator/ ./internal/run/ ./internal/verdict/` → exit 0
- `go test ./internal/verify/ ./internal/baton/... ./internal/orchestrator/ ./internal/run/ ./internal/verdict/ ./internal/scheduler/` → all `ok`
- `go test ./internal/verify/ -run RunAgentic -v` → 9 PASS (Pass/Fail/Blocked + 6 fail-closed INCONCLUSIVE cases)
- `go test ./internal/baton/ -run VerifierVerdict -v` → PASS (PASS ok; FAIL+violations ok; FAIL-without-violations rejected; bad enum rejected)
- **Full suite** `go test ./...` → PASS (run after fixing the verifier test fakes; memory rule: always run the full suite with a timeout)

## Reachability artefact
End-to-end through the integration point (`RunAgentic`), not a leaf:
- **`TestRunAgenticPass`** drives `RunAgentic` → `ChatStructured` and asserts the
  emit schema handed to the driver carries the `verifier-verdict-v1` title + the
  verdict enum, the messages are the verifier role prompt + SPEC/DIFF/PROOF, and the
  verdict/rationale/token-split come off the typed object (not prose).
- **`TestRunAgenticFailWithoutViolationsInconclusive`** is the fail-closed proof:
  a `FAIL` emitted with no violations fails `verifier-verdict-v1` validation (the
  `allOf` FAIL⇒violations≥1) and resolves to INCONCLUSIVE — a property the old
  `HasPrefix("FAIL")` scrape could never enforce.
- **`TestRunAgenticNonStructuredAgentInconclusive`** / `...MalformedEmission...` /
  `...BadVerdictEnum...` / `...StructuredDispatchError...` / `...EmptyChoices...`
  prove every other boundary fails closed to INCONCLUSIVE.
- **Loop smoke (live):** the full `go test ./internal/run/...` exercises
  `RunSlice` → `RunAgentic` end-to-end (a structured PASS drives state→verified and
  a merge commit; a garbage verifier reply fails closed and never merges —
  `TestRun_VerifyToolCallLeakBlocks`).

## Delivered
- Canonical `verifier-verdict-v1.json` schema (embedded + `SchemaMap`) — enforces
  the 4-value verdict enum and FAIL/BLOCKED⇒violations≥1. Evidence:
  `TestValidateSchema_VerifierVerdict`, `TestValidateSchema_Compiles`.
- `RunAgentic` emits via `ChatStructured` + validates via `baton.ValidateSchema`,
  fail-closed to INCONCLUSIVE on every failure boundary —
  `internal/verify/verify.go` `acceptStructuredVerdict`; the `TestRunAgentic*` suite.
- Prose scrape **deleted**: `parseVerdict` / `firstVerdictLine` / `stripMarkdown`
  removed from `verify.go`; `TestParseVerdict*` removed.
- `extractViolations` prose-split **deleted**: `slice.go` reads typed
  `lastVerdict.Violations` (schema-guaranteed non-empty for BLOCKED) + `Routing`.
- Dead stateless classifier **deleted**: `orchestrator.Interpret` et al. and the
  dead `captureVerifier`; the live INCONCLUSIVE→PAGE sentinel contract retained.
- `verdict.Result` carries typed `Violations []string` + `Routing` (kept `[]string`
  to stay off the D6/1b path).

## Not delivered (Rule 2 — why + tracking + acknowledgement)
- **No live provider dispatch.** All evidence is httptest/fake-agent. *Why:* no
  provider keys in this session. *Tracking:* same boundary as Step 2; a real
  keyed verifier dispatch is a follow-up smoke. *Ack:* surfaced here for the
  fresh verifier.
- **`inconclusive` not added to the slice-status leaf `result` enum.** *Why:*
  Option A keeps the slice bounded and off the D6/1b enforcement-flip. *Tracking:*
  **#37**. *Ack:* Brad decided 2026-06-30.
- **Routing not yet emitted by the verifier prompt.** The plumbing carries
  `routing` end-to-end (schema → `verdict.Result.Routing` → `status.json`), but the
  verifier.md role prompt is not yet updated to author it. *Why:* prompt authoring
  is its own slice. *Tracking:* folds into the `review-v1`/role-prompt sibling work
  (ADR-0011 §8 seq 3). *Ack:* here.
- **`baton-web` publishing.** `verifier-verdict-v1.json` 404s at its `$id` until
  served. *Tracking:* the cross-cutting publish task (ADR-0011 §2 / handoff). Not
  required for the in-binary authoring path (the schema is embedded).

## Divergence from plan
- Pilot retargeted from the handoff's `orchestrator.Interpret` (dead) to the live
  `verify.RunAgentic` (see continuation-handshake note above).
- Canonical schema `required` omits the identity triple vs the literal §3.3 sketch —
  forced by §3.3(g) (model payload is judgement-only); documented in the schema's
  own `description`.
- Beyond the named deletions, the dead `captureVerifier` (`slice.go`) was removed as
  part of the interpreter cleanup, and two `run_test.go` tests asserting obsolete
  prose-tolerance (`VerifyMarkdownPass`, `VerifyToolCallLeakBlocks`) were updated to
  the structured/fail-closed reality (load-bearing "not merged" assertions retained).
