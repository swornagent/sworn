# Journal — S06-model-pricing-registry

## 2026-07-01 — design_review → in_progress → implemented

**Design review outcome.** Captain review (`review.md`, commit a038449) returned
`DECISION: PROCEED` with 2 pins (1 mechanical, 1 memory-cited), 0 escalations.
Coach (Brad) acknowledged inline. Both pins addressed before implementation:

1. Added a `design_decisions` array to `status.json` recording the Type-2
   classification (schema per `S01-d6-record-reconciliation/status.json`).
2. No-consolidation scope boundary — acknowledged the
   `project_model_layer_service_refactor` memory citation; no action needed.

**AC-07 sequencing.** Filed the GitHub issue for the intro→standard price flip
*before* writing the AC-03 code comments, per the review's flag (f) —
avoided landing the `<issue/punch-list ref>` placeholder literally. Issue:
https://github.com/swornagent/sworn/issues/41.

**Implementation.** Three pricing-map edits, exactly the design's plan:
- `internal/model/pricing.go` `Pricing`: added `claude-sonnet-5: {2.00, 10.00}`;
  corrected `claude-opus-4-8` `{15.00, 75.00}` → `{5.00, 25.00}`.
- `internal/model/anthropic.go` `anthropicPricing`: same two changes.
- `internal/model/bedrock.go` `bedrockPricing`: added
  `anthropic.claude-sonnet-5: {2.00, 10.00}`; corrected
  `anthropic.claude-opus-4-8` → `{5.00, 25.00}`.
- Each `claude-sonnet-5` entry carries the AC-03 comment (both rates + expiry
  + flip instruction + issue #41).
- `PriceForModel` (client.go) needed no edit — confirmed by the design review
  as a transitive read of `anthropicPricing`/`bedrockPricing`.

**AC-05 tests.** Added `TestPricing_Sonnet5` and `TestPricing_Opus4_8CorrectedRate`
to `internal/model/pricing_test.go`, driving the real exported `ComputeCost`
(not a private map copy): sonnet-5 asserts `$12.00` for 1M/1M tokens;
opus-4-8 asserts `$30.00` (explicitly checked against the old, wrong `$90.00`).

**AC-06 audit.** Grepped `internal/model/anthropic_test.go` and
`internal/model/bedrock_test.go` for hardcoded price assertions on the two
changed models — confirmed the review's finding: the only `claude-opus-4-8`
reference (`anthropic_test.go:132`) constructs a client and asserts no price;
all hardcoded price assertions in both files cover `sonnet-4-6`/`haiku-4-5`
only. No edits needed. `go build ./...` and `go test ./internal/model/...`
both green; `go test ./...` (full suite) also green.

**Corruption-pattern check.** Grepped the touched files for the known
newline-eating edit corruption pattern (`//.*\t+(return|sendRequest|[a-z]+\()`)
per project memory — no matches.

**Proof-bundle gate divergence (Rule 2 — informational, not a deferral).**
`implementer.md`'s workflow step 5 names `sworn coverage` and
`sworn llm-check --check ac-satisfaction` / `--check security-review` as
reference-implementation gates. This build's actual CLI (`sworn --help`)
exposes no such subcommands — only `sworn verify --spec --diff --proof
--verifier-model`. Ran the gate that exists. `SWORN_ANTHROPIC_API_KEY` is
unset in this environment (matches the precedent noted in
`S04-board-record-reconciliation`'s journal — a known environment gap, not a
slice gap), so verified with `--verifier-model openrouter/z-ai/glm-5.2`
(key present in `~/.sworn/.env`). Verdict: `PASS`, cost $0 (stateless judge
gate). `--proof` does not write the file in this build (confirmed: no file
appeared at the given path); `proof.json` is authored by hand from live repo
state per Rule 6, as this journal documents.

**Out-of-scope discoveries (Rule 2):** none. No touchpoint collisions, no
spec gaps found.

State: `implemented`. Terminal state for this session — handing off to
`/verify-slice` for adversarial (fresh-context) verification.

## 2026-07-01 — Verifier verdicts received

```
PASS

Slice: `S06-model-pricing-registry`
Verified against: `ae1af3e`
Verifier session: `fresh, artefact-only`
```

Independently confirmed, from live repo state (no implementer transcript read):
- Gate 1 (reachability): `ComputeCost` and `computeAnthropicCost` are called from
  real dispatch call sites (`anthropic.go:81`, `anthropic.go:156`), not test-only
  code; `PriceForModel` reads the same corrected `anthropicPricing`/`bedrockPricing`
  maps (`client.go:78`, `client.go:86`) — the "transitive, no client.go edit
  needed" claim in proof.json checks out by inspection.
- Gate 2 (touchpoints): `git diff --name-only 102ae04a..HEAD` matches spec.json's
  4 planned touchpoints exactly (plus the 3 expected slice-artefact docs files).
- Gate 3 (tests): re-ran `go build ./...`, `go test ./internal/model/... -count=1`,
  and `go test ./... -timeout 120s -count=1` fresh (uncached) from this session —
  all green, including `TestPricing_Sonnet5` and `TestPricing_Opus4_8CorrectedRate`.
- Gate 4 (reachability artefact): matches proof.json's cited evidence; test drives
  the real exported `ComputeCost`, not a private map copy.
- Gate 5 (no silent deferrals): grepped changed files for TODO/FIXME/deferred —
  2 pre-existing "deferred" hits in `anthropic.go` (tool-use, S10 scope) sit
  entirely outside this slice's single diff hunk (confirmed via `git diff` hunk
  header `@@ -190,7 +190,11 @@`), so not attributable to S06. The one genuine
  deferral (AC-07, intro→standard price flip) is properly surfaced: GitHub issue
  #41 verified to exist via `gh api repos/swornagent/sworn/issues/41` (open,
  correct body, cites AC-07).
- Gate 6 (design conformance): no `docs/baton/design-fidelity.json` in this repo
  — non-UI project, gate auto-passes.
- Gate 7 (claimed scope): all 7 `delivered` items in proof.json independently
  verified against live repo state (diffs, test output, GH issue) — evidence
  references check out.
- Gates 3b/4b (LLM checks): `sworn llm-check` does not exist in this build's CLI
  (confirmed via `sworn --help`) and `SWORN_ANTHROPIC_API_KEY` is unset
  (confirmed via `sworn verify` exit 2) — matches the environmental-gap
  precedent from S04's journal. Non-blocking per verifier.md gate contract.

**Out-of-band observation (not a verdict input, not a slice defect):**
`gofmt -l internal/model/anthropic.go internal/model/bedrock.go` flags both
files as unformatted. Confirmed via `git show 102ae04a:<path> | gofmt -l` that
both were **already gofmt-dirty before this slice's `start_commit`** — the
violations sit outside S06's diff hunks (an unrelated `}//` spacing issue near
`anthropicStatusCode`, a missing trailing newline, and a fused `Verify(...) {`
signature+statement in `bedrock.go` resembling the newline-eating corruption
pattern in project memory). CI's `gofmt` gate (`.github/workflows/ci.yml`,
`release.yml`) will fail on this branch regardless of S06. Not in scope for
this slice to fix (S06's own edits are gofmt-clean by omission — the map diff
itself doesn't introduce new violations) and not raised as a Gate 5 deferral
since it predates S06 entirely. Flagging for a human decision on tracking.

```
STATE: verified_awaiting_approval
SLICE: `S06-model-pricing-registry`
NEXT: /merge-track T5-model-pricing-registry 2026-06-30-sworn-operational-readiness
REASON: All six gates passed. Track `T5-model-pricing-registry` is complete — S06 is its only slice.
```
