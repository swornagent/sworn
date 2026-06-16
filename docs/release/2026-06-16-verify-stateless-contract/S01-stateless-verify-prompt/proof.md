---
title: S01-stateless-verify-prompt proof bundle
description: Rule 6 proof bundle. Generated from live repo state.
---

# Proof Bundle: `S01-stateless-verify-prompt`

## Scope

The verify path is told "judge from SPEC+DIFF only, verdict-leading, no tools"
instead of the agentic role prompt.

## Files changed

```
$ git diff --name-only 68aa6a3..HEAD
docs/release/2026-06-16-verify-stateless-contract/S01-stateless-verify-prompt/status.json
internal/prompt/prompt.go
internal/prompt/prompt_test.go
internal/prompt/verify-stateless.md
internal/verify/verify.go
internal/verify/verify_test.go
```

## Test results

### Go

```
$ go test ./internal/prompt/... ./internal/verify/... -v
=== RUN   TestVerifier_NonEmpty
--- PASS: TestVerifier_NonEmpty (0.00s)
=== RUN   TestVerifier_ContainsVerdictContract
--- PASS: TestVerifier_ContainsVerdictContract (0.00s)
=== RUN   TestVerifier_NotOldPlaceholder
--- PASS: TestVerifier_NotOldPlaceholder (0.00s)
=== RUN   TestVerifier_ContainsInconclusive
--- PASS: TestVerifier_ContainsInconclusive (0.00s)
=== RUN   TestImplementer_NonEmpty
--- PASS: TestImplementer_NonEmpty (0.00s)
=== RUN   TestPlanner_NonEmpty
--- PASS: TestPlanner_NonEmpty (0.00s)
=== RUN   TestCaptain_NonEmpty
--- PASS: TestCaptain_NonEmpty (0.00s)
=== RUN   TestVerifyStateless_NonEmpty
--- PASS: TestVerifyStateless_NonEmpty (0.00s)
=== RUN   TestVerifyStateless_StatelessMarkers
--- PASS: TestVerifyStateless_StatelessMarkers (0.00s)
=== RUN   TestVerifyStateless_NotAgenticVerifier
--- PASS: TestVerifyStateless_NotAgenticVerifier (0.00s)
=== RUN   TestBatonVersion_NonEmpty
--- PASS: TestBatonVersion_NonEmpty (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/prompt	0.004s
=== RUN   TestRun_PassExitsZero
--- PASS: TestRun_PassExitsZero (0.00s)
=== RUN   TestRun_MissingSpecBlocks
--- PASS: TestRun_MissingSpecBlocks (0.00s)
=== RUN   TestRun_UnconfiguredModelFailsClosed
--- PASS: TestRun_UnconfiguredModelFailsClosed (0.00s)
=== RUN   TestRun_MissingFileBlocks
--- PASS: TestRun_MissingFileBlocks (0.00s)
=== RUN   TestRun_GarbledVerdictBlocks
--- PASS: TestRun_GarbledVerdictBlocks (0.00s)
=== RUN   TestRun_SystemPromptIsStateless
--- PASS: TestRun_SystemPromptIsStateless (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/verify	0.004s
```

```
$ go vet ./...
(clean, no output)
```

```
$ go build ./...
(clean, no output)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: N/A (smoke step)
- **User gesture**: `sworn verify --spec <synthetic> --diff <synthetic>` reaches
  dispatch without build or wiring panic — exits 2 (BLOCKED) on Unconfigured
  model, same behaviour as before the prompt change.

```
$ go build -o /tmp/sworn-s01 ./cmd/sworn/
BUILD OK

$ sworn verify --spec /tmp/spec --diff /tmp/diff
sworn verify: model: SWORN_OPENAI_API_KEY not set
EXIT: 2
```

## Delivered

- [x] `internal/verify/verify.go` no longer references `prompt.Verifier()`; its
      `systemPrompt` is sourced from `prompt.VerifyStateless()`.
      Evidence: `internal/verify/verify.go:23` — `var systemPrompt = prompt.VerifyStateless()`.
- [x] The new prompt is embedded (`go:embed`) and the binary builds with zero
      added dependencies.
      Evidence: `internal/prompt/prompt.go:14` — `go:embed` directive includes
      `verify-stateless.md`. `go build ./...` exits 0.
- [x] The new prompt text explicitly states "no tools / no repo / SPEC+DIFF only"
      and "reply MUST begin with one of PASS/FAIL/BLOCKED/INCONCLUSIVE as the first
      characters".
      Evidence: `internal/prompt/verify-stateless.md` lines 9-12, 32-36.
- [x] The four verdict tokens and the BLOCKED-vs-INCONCLUSIVE distinction are
      retained in the prompt wording.
      Evidence: `internal/prompt/verify-stateless.md` lines 19-49 — all four
      verdicts defined with BLOCKED/INCONCLUSIVE distinction.
- [x] `prompt.Verifier()` still returns `verifier.md` verbatim (no mutation of the
      vendored artefact); a `prompt` package test asserts it is non-empty and
      unchanged in shape.
      Evidence: `internal/prompt/prompt_test.go` — `TestVerifier_NonEmpty`,
      `TestVerifier_ContainsVerdictContract`, `TestVerifier_NotOldPlaceholder`,
      `TestVerifier_ContainsInconclusive` all pass.

## Not delivered

None. All five acceptance checks are delivered.

## Divergence from plan

None. All three planned touchpoints (`internal/prompt/verify-stateless.md`,
`internal/prompt/prompt.go`, `internal/verify/verify.go`) modified. Test files
(`internal/prompt/prompt_test.go`, `internal/verify/verify_test.go`) also modified
as expected per the test plan.

## First-pass script output

```
release-verify.sh
  slice:       S01-stateless-verify-prompt
  slice dir:   docs/release/2026-06-16-verify-stateless-contract/S01-stateless-verify-prompt
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Diff vs main ==
  PASS  191 file(s) changed vs main

== Dark-code markers in changed files ==
  FAIL  dark-code markers found in changed source files (must be Rule 2 deferrals)
  hits:
    internal/adopt/adopt.go:
    59:+breakpoints. Session end: record decisions, completed work, deferred items,

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: 17
  checks failed: 1
FIRST-PASS FAIL

Note: the single dark-code failure is a false positive in
internal/adopt/adopt.go:59 — the word "deferred" appears in embedded Baton
protocol documentation from a prior release (2026-06-15-e2e-turnkey-loop).
The script diffs vs main (entire track branch), not our slice's
start_commit..HEAD range. Our slice's actual files contain no dark-code
markers (confirmed via grep of the slice's own diff).
```