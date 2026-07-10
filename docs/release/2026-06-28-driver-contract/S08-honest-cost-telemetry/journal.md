# Journal — S08-honest-cost-telemetry

## 2026-07-11 — Implementer session: design_review → implemented

**Entry state.** Coach acknowledgement already committed (`captain-proceed.md`
@4753eb3, verdict PROCEED) on `track/2026-06-28-driver-contract/T4-resolution-loop`.
Verified the ack exists and cites the exact commit before starting (Rule 7
handoff discipline — the implementer does not re-litigate a design review).

**Design_decisions recorded before code (Rule 9 gate).** `status.json` had no
`design_decisions` field despite review.md pins 1–2 requiring Type-1 Coach
records. Added D1/D2 (both Type-1, Coach-ratified fail-closed) and D3 (Type-2,
confirmed-as-designed) before transitioning to `in_progress`, citing
captain-proceed.md@4753eb3 as `decision_ref`.

- **D1/D2 fail-closed ruling.** The task brief's Coach ruling was stricter
  than design.md's own proposal: design.md proposed implementing a
  `TotalCostUSD==0 -> "subscription"` inference for claude.go (D1 in the
  design) and a `Usage!=nil -> "subscription"` inference for codex.go (D2 in
  the design), flagging both as "informed guesses... not verified against a
  live binary" and asking the Captain/Coach to ratify. The Coach ratified the
  OPPOSITE of the design's proposal: ship `CostSourceUnknown` universally for
  both — no positively identified, testable marker exists in the currently
  observed CLI output for either binary, so neither inference is implemented.
  This is the binding instruction for this session (task brief pins 1/2) and
  is what got built.

**AC-02 "provider" branch.** Confirmed already amended on this branch
(`aaa2861` replan commit, forward-merged via `053e624`) before this session
started — spec.json's AC-02 text already reads "reserved... no live dispatch
path claims CostSource=provider in this slice." No spec.json edit was needed
this session; implementation follows the amended text as-is.

**Implementation order.**

1. `internal/driver/driver.go` — added `CostSource*` named constants (D3),
   fixed the stale `Result.CostSource` doc comment.
2. AC-01: deleted `internal/model/pricing.go` (the `Pricing` map/`ComputeCost`
   func); redirected `anthropic.go` (`Verify`+`Chat`), `oai.go` (`Verify`),
   `openai_responses.go` (`Verify`) to `ComputeCostFromTokens`; deleted the
   now-dead `computeAnthropicCost` and `oai.go`'s local `computeCost`.
   Rewrote `pricing_test.go` against the surviving API and added
   `TestPricingUnified` (enumerates every model key across all four provider
   maps, asserts `PriceForModel` resolves each to the exact source-map rate —
   the R-01 "one path, no drift" regression guard).
3. `internal/agent/agent.go` — deleted the flat-rate `computeCost` and
   dropped the `float64` cost return from `Run`'s signature (pre-authorised
   per the task brief; blast radius independently re-confirmed at exactly the
   two call sites `inprocess.go:183` and `inprocess_verify.go:43`, both
   already discarding the value). Updated all 7 call sites in
   `agent_test.go`.
4. AC-02: rewrote `inprocess.go`'s `economics()` to compute `CostUSD` from
   the CONFIRMED response model-id (`meter.modelID`) and the true
   accumulated token split via `model.PriceForModel`/`ComputeCostFromTokens`;
   `CostSource=pricing-table` on a hit, `CostSource=unknown` + a stderr
   warning naming the model on a miss (AC-04). Deleted
   `nominalCostPerToken`. The `"provider"` branch is deliberately NOT
   implemented — no wired client returns a real provider-reported figure
   today; an unreachable branch would be untestable dead code.
5. AC-03: rewrote `claude.go`'s `costSource()` to the D1 ruling — only a
   strictly positive `TotalCostUSD` earns `CostSourceCLI`; nil OR an explicit
   zero both classify `CostSourceUnknown`. Rewrote `codex.go`'s
   `costSource()` to the D2 ruling — always `CostSourceUnknown` (codex never
   carries a cost field in any documented shape, and `Usage!=nil` is not a
   positively identified subscription marker). Added a new
   `fakeClaudeZeroCost` fixture + `TestClaudeEnvelopeExplicitZeroCostIsUnknown`
   to prove D1's ambiguous-zero case explicitly (the pre-existing minimal-
   envelope test only covered the nil case).
6. AC-05: `driver.Result.CostSource` already existed and is populated by
   steps 4/5 above — no driver-side struct change needed here. Added
   `CostSource` to `verdict.Result` and `state.Dispatch` (additive,
   `omitempty`); threaded it through
   `verify.go`'s `withDispatchEconomics` and `acceptStructuredVerdict`, and
   through all 5 `state.Dispatch{}` construction sites in `slice.go`
   (captain, implementer, verifier carry it from their driver.Result /
   verdict.Result source; the two synthetic zero-cost sites — proof-absent,
   first-pass — correctly leave it empty, since no driver call was made).
   Added `TestRunSlice_CostSourceThreadedToStatusJSON` (Rule 1 reachability
   artefact — drives the real `RunSlice` entry point, not a leaf
   `state.Dispatch` struct literal) proving two distinct `CostSource` values
   reach the written `status.json` and that the file still validates against
   `slice-status-v1`.
7. Updated the pre-existing `"estimated"`/`"provider-reported"` fixtures in
   `captain/review_test.go` and `verify/verify_agentic_test.go` to the new
   named-constant vocabulary (not required for those tests to pass, but the
   old strings are no longer valid vocabulary members).

**Full-suite proof.** `go test -count=1 -timeout 300s ./...` — all 45
packages green, zero regressions in any package this slice did not touch
(the project's known newline-eating-edit hazard: always confirmed via the
full suite, not the named-package subset alone).

**Rule 2 deferrals recorded in proof.json `not_delivered`:** D1's
subscription inference (claude.go), D2's subscription inference (codex.go),
sworn#89 (Google/Bedrock pricing duplication — already filed and
acknowledged at design review, carried forward here).

**State transition:** `design_review` → `in_progress` (this session, commit
f8ca6b5) → `implemented` (this session, closing commit below).

## Verifier verdicts received

### 2026-07-11 — fresh-context verifier (round 1)

FAIL

Slice: `S08-honest-cost-telemetry`

Violations:
1. Gate 3 — Amended AC-02 (Coach, 2026-07-10) requires the reserved
   CostSource="provider" vocabulary to exist "as a named constant with a
   contract test". The constant `driver.CostSourceProviderReported` exists
   (internal/driver/driver.go:162) and correctly has ZERO live emission
   sites (verified by repo-wide grep — no production path claims it), but
   NO contract test exists anywhere in the repo: no test file references
   `CostSourceProviderReported` or asserts its "provider" value.
   proof.json's delivered item concedes it substituted "the pre-existing
   amended AC-02 spec text as its contract" — spec prose is not a test.
   Evidence: internal/driver/driver.go:162; `grep -rn
   CostSourceProviderReported --include='*_test.go'` returns zero hits;
   proof.json delivered (AC-02 item).

Required to address:
1. Add a contract test in internal/driver (e.g. TestCostSourceVocabulary)
   pinning each CostSource* constant's persisted string value — including
   `CostSourceProviderReported == "provider"` — so the reserved vocabulary
   member cannot drift or be silently claimed; re-run
   `go test ./internal/driver/...` and the full suite.

Verified green (for the record): pricing unification complete — all 15
deleted-table models resolve via PriceForModel at identical rates
(TestPricingUnified re-run PASS); no flat-rate cost function remains
anywhere; D1/D2 fail-closed rulings correctly implemented (claude explicit
zero -> unknown, codex always unknown, no fabricated figure, no
subscription inference); cost_source threaded end-to-end into the WRITTEN
status.json validating slice-status-v1
(TestRunSlice_CostSourceThreadedToStatusJSON re-run PASS); full suite
`go test -count=1 -timeout 300s ./...` — 45 packages ok, exit 0.

## 2026-07-11 — Implementer re-entry session: failed_verification → implemented

**Scope: exactly the one numbered violation from the round-1 verifier, nothing
else.** `start_commit` (f8ca6b5) left untouched per the S02b/S01
re-entry lesson (feedback_start_commit_reentry;
project_2026_07_02...cf72bcd precedent) — the historical FAIL
`verification` block in `status.json` is also left untouched, not
overwritten, for the same reason: it is the durable record of round 1, not a
scratch field.

**Fix.** Added `internal/driver/driver_test.go:TestCostSourceVocabulary`, a
table-driven contract test pinning all five `CostSource*` constants'
persisted string values (`driver.go:162` onward), including
`CostSourceProviderReported == "provider"` — the reserved vocabulary member
the round-1 verifier found had zero test references anywhere in the repo.
This directly answers Gate 3: proof.json's first-pass AC-02 delivered item
had substituted the amended spec text itself as the "contract", which the
verifier correctly rejected as prose, not a test.

No other file touched. Verified via `git diff f8ca6b5..HEAD --stat` that
`internal/driver/driver_test.go` is the only file changed relative to the
round-1 commit (all round-1 files land unchanged in the diff-vs-start_commit
already captured in status.json `actual_files`).

**Checks run this session.**
- `grep -n '//.*\t+(return|sendRequest|[a-z]+\()' internal/driver/driver_test.go`
  — zero hits (newline-eating-edit hazard check, per project hazards).
- `gofmt -l internal/driver/driver_test.go` — clean.
- `go vet ./internal/driver/...` — clean.
- `go test -count=1 -v -run TestCostSourceVocabulary ./internal/driver/...` —
  PASS, 5/5 subtests.
- `go test -count=1 ./internal/driver/...` — PASS (full package, no
  regression from the new test).
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test -count=1 -timeout 300s ./...` — 45 packages ok, exit 0 (full
  suite re-run, no regressions anywhere).

**proof.json / proof.md updated** to add `internal/driver/driver_test.go` to
`files_changed`/`actual_files`, add the new test command and reachability
line, and amend AC-02's `delivered` evidence to cite
`TestCostSourceVocabulary` instead of the rejected spec-prose substitute. A
`divergence` entry records the re-entry scope explicitly.

**State transition:** `failed_verification` → `implemented` (this session).
Historical round-1 FAIL verdict preserved verbatim in `status.json`
`verification` and in this journal's "Verifier verdicts received" section
above, per Rule 6/Rule 7 (the proof bundle records live state, but does not
erase the verification history that got it there).
