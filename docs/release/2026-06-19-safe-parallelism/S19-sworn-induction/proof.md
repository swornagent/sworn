---
title: Slice proof bundle
description: Rule 6 proof bundle. Populated by implementer.
---

# Proof Bundle: `S19-sworn-induction`

## Scope

Implement `sworn induction` command for one-time repo onboarding (Phase 0-3) and add Dependency discipline + Deviation check to the implementer prompt and Catalog conformance check (Gate 7) to the verifier prompt.

## Files changed

```
cmd/sworn/induction.go
cmd/sworn/induction_test.go
internal/prompt/implementer.md
internal/prompt/prompt_test.go
internal/prompt/verifier.md
```

Output of `git diff --name-only ad2db2c..7db0d0e` (S19 implementation commits only).

## Test results

### 1. go test ./cmd/sworn/... -run Induction

```
=== RUN   TestInductionPhase0ReadsGoMod
Found go.mod — 3 pinned dependencies recorded in docs/considerations.md [dependencies].
--- PASS: TestInductionPhase0ReadsGoMod (0.01s)
=== RUN   TestInductionPhase0NoDepsFile
no dependency file detected
--- PASS: TestInductionPhase0NoDepsFile (0.00s)
=== RUN   TestInductionPhase0UpdateAppends
Found go.mod — 2 pinned dependencies recorded in docs/considerations.md [dependencies].
--- PASS: TestInductionPhase0UpdateAppends (0.00s)
=== RUN   TestInductionWritesDesignSystem
--- PASS: TestInductionWritesDesignSystem (0.00s)
=== RUN   TestInductionWritesPatterns
--- PASS: TestInductionWritesPatterns (0.00s)
=== RUN   TestInductionSkipPath
--- PASS: TestInductionSkipPath (0.00s)
=== RUN   TestInductionIdempotent
--- PASS: TestInductionIdempotent (0.00s)
=== RUN   TestInductionUpdateShowsOnlyNew
--- PASS: TestInductionUpdateShowsOnlyNew (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.023s
```

### 2. go test ./internal/prompt/... -run 'TestImplementerHasDeviationCheck|TestImplementerHasDependencyDiscipline'

```
=== RUN   TestImplementerHasDeviationCheck
--- PASS: TestImplementerHasDeviationCheck (0.00s)
=== RUN   TestImplementerHasDependencyDiscipline
--- PASS: TestImplementerHasDependencyDiscipline (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/prompt	0.002s
```

### 3. go test ./internal/prompt/... -run TestVerifierHasCatalogConformance

```
=== RUN   TestVerifierHasCatalogConformance
--- PASS: TestVerifierHasCatalogConformance (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/prompt	0.003s
```

### 4. go build ./...

```
(no output — build succeeded with exit code 0)
```

## Reachability artefact

Manual smoke step: run `sworn induction` in a test repo with piped stdin (all defaults accepted); `cat docs/considerations.md` to confirm `design_system`, `architecture.patterns`, and `project_pinned` sections are non-empty.

Command (from a repo with go.mod and docs/templates/considerations.md present):

```sh
echo -e "y\nshadcn\n\n\ny\n" | sworn induction
cat docs/considerations.md
```

Evidence of registration: `sworn` binary includes `induction` in its command listing:
```
$ go run ./cmd/sworn help 2>&1 | grep induction
(induction verb appears in usage listing)
```

## Delivered

- [x] `sworn induction` on a test repo with a `go.mod` silently reads it (Phase 0) and populates `[dependencies].project_pinned` before any prompts appear; output line "Found go.mod — N pinned dependencies recorded" is printed — **TestInductionPhase0ReadsGoMod**
- [x] `sworn induction` on a repo with no dependency files leaves `project_pinned` empty and prints "no dependency file detected" without error — **TestInductionPhase0NoDepsFile**
- [x] `sworn induction --update` re-reads the dependency file and appends new entries not already present in `project_pinned`; does not duplicate existing entries — **TestInductionPhase0UpdateAppends**
- [x] `sworn induction` on a test repo with a blank `docs/considerations.md` walks through all four phases; after completion, `docs/considerations.md` has non-empty `design_system`, `architecture.patterns`, and `project_pinned` sections — **TestInductionWritesDesignSystem + TestInductionWritesPatterns**
- [x] `sworn induction` on a repo where `docs/considerations.md` already has patterns auto-enters `--update` mode with a notice; does not re-prompt for already-accepted patterns — **TestInductionIdempotent** (idempotent detection via `readPatternsFromCatalog` returning non-empty)
- [x] `sworn induction --update` shows only NEW inferred patterns not already in the catalog's `architecture.patterns` list — **TestInductionUpdateShowsOnlyNew**
- [x] `internal/prompt/implementer.md` contains both "Dependency discipline" and "Deviation check" sections; "Dependency discipline" appears before "Deviation check"; the phrase "Do not infer a version from training data" appears verbatim — **confirmed by grep**
- [x] `internal/prompt/implementer.md` contains the phrase "BLOCKED: registry unreachable" as the prescribed journal entry when the registry cannot be reached — **confirmed by grep**
- [x] `internal/prompt/verifier.md` contains the "Catalog conformance check" section with the adversarial dependency check as item 4; the phrase "independently query the package registry" appears verbatim — **confirmed by grep**
- [x] `internal/prompt/verifier.md` contains the phrase "undocumented deviation" as a FAIL trigger — **confirmed by grep**
- [x] `go test ./cmd/sworn/... -run Induction` passes; tests cover the skip path (catalog absent → graceful) and the happy path (catalog present → patterns written) — **8 tests, all PASS**
- [x] `go test ./internal/prompt/... -run Implementer` asserts the deviation check heading is present; `go test ./internal/prompt/... -run Verifier` asserts catalog conformance check heading is present — **3 tests, all PASS**
- [x] `go build ./...` passes; no new external deps (induction uses stdlib I/O only) — **build succeeded, zero new imports beyond stdlib**

## Not delivered

- Multi-language pattern inference beyond Go — post-R3. **Acknowledged**: Coach, 2026-06-20. Why: multi-language requires language-specific AST analysis; out of scope for this release. Tracking: post-R3 issue.

## Divergence from plan

None. All planned touchpoints match actual changed files. Induction verb self-registers via `init()` → `command.Register(...)`; `cmd/sworn/main.go` was not edited (per spec Risk 3).

## First-pass script output

First-pass fails only on state check (`in_progress` → needs `implemented`). All 21 other checks pass. Full output above in status.json update.