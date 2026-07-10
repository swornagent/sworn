# Proof bundle — S08-honest-cost-telemetry

Rendered from `proof.json` (schema `proof-v1`). See that file for the
machine-readable record; this is the human-readable summary.

## Scope

Every dispatch record carries honest economics — real cost computed from the
CONFIRMED model's pricing or carried from the CLI's own report, an explicit
`cost_source`, true token split, duration, and confirmed model-id — so a
subscription dispatch is never recorded as fake $0 API spend, and no cost
figure is ever guessed or fabricated.

## Files changed

27 files — see `proof.json` `files_changed` for the full list. Summary:
`internal/model/pricing.go` deleted; `internal/model/{anthropic,oai,openai_responses,pricing_test}.go`,
`internal/agent/agent.go` (+test), `internal/driver/{driver,claude,codex,subprocess_test}.go`
(+tests), `internal/driver/inprocess/*.go` (+tests), `internal/verdict/verdict.go`,
`internal/verify/verify.go` (+test), `internal/state/state.go` (+test),
`internal/run/slice.go` (+test), `internal/captain/review_test.go`. Re-entry
session (addressing verifier round-1 Gate 3): `internal/driver/driver_test.go`
gained `TestCostSourceVocabulary` — no other production or test file touched.

## Test results

| Command | Result |
|---|---|
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `gofmt -l <every changed .go file, incl. driver_test.go>` | PASS (clean) |
| `go test -count=1 -v -run TestCostSourceVocabulary ./internal/driver/...` (re-entry session, closes verifier round-1 Gate 3) | PASS — 5 subtests, pins all `CostSource*` constants incl. `CostSourceProviderReported == "provider"` |
| `go test -count=1 ./internal/model/... ./internal/agent/... ./internal/state/... ./internal/run/... ./internal/driver/... ./internal/verify/... ./internal/verdict/... ./internal/captain/...` (AC-05's named command + design.md's additional touched packages) | PASS |
| `go test -count=1 -timeout 300s ./...` (full suite, re-run after the round-1 fix) | PASS — 45 packages ok, 0 FAIL |

## Reachability artefact

`go test -count=1 -v -run TestRunSlice_CostSourceThreadedToStatusJSON ./internal/run/` — PASS.

Drives the real `RunSlice` engine entry point (the same function the
implementer/verifier loop calls in production), not a leaf `state.Dispatch`
struct literal: a fake driver's implementer leg reports
`CostSource=pricing-table` and its verifier leg reports `CostSource=cli`;
`RunSlice` runs the full implement → verify → transition-to-verified path,
and the test reads the **written status.json off disk afterward**, asserting
`verification.dispatches[].cost_source` carries each role's distinct value
through unmodified, and that the resulting file still validates against
`slice-status-v1`.

Two supporting non-leaf-adjacent proofs:

- `TestClaudeEnvelopeExplicitZeroCostIsUnknown` (`internal/driver/claude_test.go`)
  drives `ClaudeDriver.Dispatch` through a real fake-CLI subprocess spawn with
  an envelope carrying an explicit reported zero cost — proving the D1 ruling
  end to end through the actual subprocess spawn/parse path.
- `TestInprocessImplementerPricingTable` (`internal/driver/inprocess/inprocess_test.go`)
  drives `InProcess.Dispatch` against a real `httptest` server returning a
  priced model ID — proving AC-02's happy path computes `CostUSD` from the
  true accumulated token split via the pricing registry, through the actual
  `chatMeter`/`agent.Run` loop.

Plus, added this re-entry session to close verifier round-1 Gate 3:
`TestCostSourceVocabulary` (`internal/driver/driver_test.go`) — a table-driven
contract test pinning all five `CostSource*` constants' persisted string
values, including `CostSourceProviderReported == "provider"`, the reserved
vocabulary member the round-1 verifier found had no contract test anywhere in
the repo (spec prose cited in AC-02 is not a test).

## Delivered

See `proof.json` `delivered` for the full per-AC breakdown with evidence
citations (AC-01 through AC-05, D3, D1/D2 recorded as Coach-ratified Type-1
`design_decisions`). AC-02's evidence now includes
`internal/driver/driver_test.go:TestCostSourceVocabulary`.

## Not delivered

- **D1** (claude.go `TotalCostUSD==0 -> subscription` inference) — not
  implemented. No positively identified, testable marker in the currently
  observed claude-cli output distinguishes an explicit-zero-because-
  subscription from an explicit-zero-because-genuinely-free-turn. Ships
  `CostSourceUnknown` instead. Tracking: `status.json` `design_decisions.D1`.
  Acknowledgement: Coach-ratified fail-closed, `captain-proceed.md@4753eb3`
  pin 1, restated in this session's task brief.
- **D2** (codex.go `Usage!=nil -> subscription` inference) — not implemented.
  `Usage!=nil` only proves a turn completed, not that it was
  subscription-covered rather than API-billed; `codexEnvelope` carries no
  distinguishing field. Ships `CostSourceUnknown` universally. Tracking:
  `status.json` `design_decisions.D2`. Acknowledgement: same as D1.
- **sworn#89** (Google/Bedrock pricing-lookup duplication) — out of scope,
  filed and acknowledged at design review (`review.md` pin 3, all three
  Rule 2 legs present). Carried forward unchanged.

## Divergence from plan

See `proof.json` `divergence` for the full text. Summary: touchpoints
expanded beyond spec.json's original list (design.md's grounding found the
real pricing lookups and the AC-05 thread's full footprint before code was
written; reviewed at design review); D1/D2's shipped behaviour is the
opposite of design.md's own proposal, because the Coach's ratification
overrode the design's proposed inference in favour of the stricter
fail-closed posture (the expected outcome of the design_review gate, not an
implementer-session deviation); the test command run was widened beyond
AC-05's literal string to the additional touched packages design.md's own
traceability section names. Re-entry session (addressing verifier round-1
FAIL, Gate 3 only): the first-pass proof.json had substituted amended AC-02's
own spec text as the `CostSource="provider"` vocabulary's contract — the
fresh verifier correctly rejected spec prose as not a test. Fix scoped
exactly to the one required violation (`TestCostSourceVocabulary`); no other
file touched; `start_commit` unchanged; full suite re-run green.
