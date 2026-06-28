# Proof bundle — S19-captain-split

## Scope

Split `internal/prompt/captain.md` into `design-reviewer.md` (design-review function) and `orchestrator-notes.md` (documents that the release-orchestrator function is realised by the Sworn engine). Update captain.md with a split-notice header. `prompt.Captain()` continues to work without breakage.

## Files changed

```
docs/release/2026-06-27-conformance-foundation/S19-captain-split/journal.md   |  26 ++
docs/release/2026-06-27-conformance-foundation/S19-captain-split/status.json  |   2 +-
internal/prompt/captain.md                                                     |  16 +-
internal/prompt/design-reviewer.md                                             | 294 +++++++++++
internal/prompt/orchestrator-notes.md                                          |  63 +++
5 files changed, 397 insertions(+), 4 deletions(-)
```

## Test results

```
$ go test ./internal/prompt/ -v -count=1
=== RUN   TestVerifier_NonEmpty — PASS
=== RUN   TestVerifier_ContainsVerdictContract — PASS
=== RUN   TestVerifier_NotOldPlaceholder — PASS
=== RUN   TestVerifier_ContainsInconclusive — PASS
=== RUN   TestImplementer_NonEmpty — PASS
=== RUN   TestPlanner_NonEmpty — PASS
=== RUN   TestCaptain_NonEmpty — PASS
=== RUN   TestVerifyStateless_NonEmpty — PASS
=== RUN   TestVerifyStateless_StatelessMarkers — PASS
=== RUN   TestVerifyStateless_NotAgenticVerifier — PASS
=== RUN   TestCaptain_ResolveDirtyWorktree — PASS
=== RUN   TestBatonVersion_NonEmpty — PASS
=== RUN   TestPlannerHasPhase2b — PASS
=== RUN   TestPlannerPhase2bDRYGate — PASS
=== RUN   TestPlannerPhase2bFastPath — PASS
=== RUN   TestImplementerHasDeviationCheck — PASS
=== RUN   TestImplementerHasDependencyDiscipline — PASS
=== RUN   TestVerifierHasCatalogConformance — PASS
=== RUN   TestBatonRulesNonEmpty — PASS
=== RUN   TestBatonAllKeys — PASS
=== RUN   TestBatonRulesHasAllTen — PASS
=== RUN   TestBatonMissingFile — PASS
=== RUN   TestEmbeddedPromptsPublicSafe — PASS
=== RUN   TestCaptainKeepsRoleVocab — PASS
PASS
ok  	github.com/swornagent/sworn/internal/prompt	0.004s
```

## Reachability artefact

### AC1 — design-reviewer.md exists and contains design-review content

```
$ wc -l internal/prompt/design-reviewer.md
293 internal/prompt/design-reviewer.md

$ head -3 internal/prompt/design-reviewer.md
# Design Reviewer role
You are the **Design Reviewer** — the Captain in its design-review capacity.
```

Contains: Step 1–6 review function, pin surfacing logic (`[mechanical]`, `[memory-cited]`, `[escalate]`), stakes classification (Step 2b — Design-fit gate Rule 9 check).

### AC2 — orchestrator-notes.md exists and states orchestrator is realised by Sworn engine

```
$ wc -l internal/prompt/orchestrator-notes.md
62 internal/prompt/orchestrator-notes.md

$ grep "realised by the Sworn" internal/prompt/orchestrator-notes.md
**The release-orchestrator function is realised by the Sworn engine, not by a
```

### AC3 — captain.md contains split notice header

```
$ grep "Split notice" internal/prompt/captain.md
> **Split notice (S19-captain-split):** This file is being split. The design-review
```

### AC4 — prompt.Captain() returns design-review content

```
$ go test ./internal/prompt/ -v -run "TestCaptain_ResolveDirtyWorktree"
--- PASS: TestCaptain_ResolveDirtyWorktree (0.00s)
# This test verifies Captain() contains "design-review" and "Step 1 — Drift detection"
```

### AC5 — "release orchestrator" conflating language removed

```
$ grep -n "release orchestrator" internal/prompt/captain.md
(no matches — conflating phrase removed; annotated as "release-orchestrator function" in split notice)
```

## Delivered

1. ✅ `internal/prompt/design-reviewer.md` — 293 lines, self-contained design-review role prompt with six-step review, pin surfacing, stakes classification
2. ✅ `internal/prompt/orchestrator-notes.md` — 62 lines, states orchestrator function is realised by Sworn engine, cross-references S18 docs
3. ✅ `internal/prompt/captain.md` — updated with split-notice header; "release orchestrator" conflating language removed
4. ✅ `prompt.Captain()` continues to work — all captain tests pass (TestCaptain_NonEmpty, TestCaptain_ResolveDirtyWorktree, TestCaptainKeepsRoleVocab, TestEmbeddedPromptsPublicSafe)
5. ✅ `grep -n "release orchestrator" internal/prompt/captain.md` returns empty — conflating language removed

## Not delivered

(None — all acceptance checks satisfied.)

## Divergence from plan

(None — implementation follows the spec exactly. Kept captain.md intact with header note per spec Risks guidance, no Go code changes needed since `internal/captain/` package does not exist.)