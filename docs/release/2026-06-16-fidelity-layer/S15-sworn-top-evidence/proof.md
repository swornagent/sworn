# Proof Bundle: S15-sworn-top-evidence

> Generated from live repo state, not recollection.

## Scope

When a maintainer runs `sworn top`, sworn renders a read-only evidence surface for the active release: each critical journey in scope with its validation status (un-walked / walked-pass / walked-fail), assembled into a green-board when all pass and a kill-list when any fail. The surface only reads and displays; it issues no state transitions and gates nothing.

## Files changed

```
$ git diff --name-only e3b0ec2..HEAD
cmd/sworn/main.go
cmd/sworn/top.go
cmd/sworn/top_test.go
docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/journal.md
docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/proof.md
docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/journey/walkthrough.go
internal/journey/walkthrough_test.go
```

(`docs/release/2026-06-16-fidelity-layer/index.md` also changed — this is the track worktree registration from Step 0)

> **Note on re-entry**: This slice was initially implemented with `start_commit: a58733d` (the implementation commit itself), which caused `git diff --name-only <start_commit>..HEAD` to return only doc files. The correct start_commit is `e3b0ec2` (the materialise-commit immediately before the first implementation commit), and the corrected diff above shows all planned touchpoints.

## Test results

### Go

```
$ go test ./cmd/sworn/ -run TestTop -v
=== RUN   TestTop_EmptyState
--- PASS: TestTop_EmptyState (0.00s)
=== RUN   TestTop_GreenBoard
--- PASS: TestTop_GreenBoard (0.00s)
=== RUN   TestTop_KillList_Unwalked
--- PASS: TestTop_KillList_Unwalked (0.00s)
=== RUN   TestTop_KillList_Failed
--- PASS: TestTop_KillList_Failed (0.00s)
=== RUN   TestTop_ReadOnly
--- PASS: TestTop_ReadOnly (0.00s)
=== RUN   TestTop_Mixed
--- PASS: TestTop_Mixed (0.00s)
=== RUN   TestTop_EmptyJourneysArtefact
--- PASS: TestTop_EmptyJourneysArtefact (0.00s)
=== RUN   TestTopCmd_MixedStatuses
--- PASS: TestTopCmd_MixedStatuses (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.009s

$ go test ./internal/journey/ -run "TestLoadAttest|TestAttest" -v
=== RUN   TestLoadAttestations_MissingFile
--- PASS: TestLoadAttestations_MissingFile (0.00s)
=== RUN   TestLoadAttestations_ExistingFile
--- PASS: TestLoadAttestations_ExistingFile (0.00s)
=== RUN   TestLoadAttestations_InvalidJSON
--- PASS: TestLoadAttestations_InvalidJSON (0.00s)
=== RUN   TestAttestationStatus_NoArtefact
--- PASS: TestAttestationStatus_NoArtefact (0.00s)
=== RUN   TestAttestationStatus_NoMatch
--- PASS: TestAttestationStatus_NoMatch (0.00s)
=== RUN   TestAttestationStatus_Match
--- PASS: TestAttestationStatus_Match (0.00s)
=== RUN   TestAttestationArtefactPath
--- PASS: TestAttestationArtefactPath (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/journey	0.004s

$ go vet ./...
(clean)
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **User gesture**:
  1. Create a test project with `.sworn/journeys.json` containing one journey and no attestations.
  2. Run `sworn top test-release /path/to/project`.
  3. Observe the kill-list output showing the journey as "un-walked".
  4. Create `.sworn/attestations.json` with a walked-pass attestation for that journey.
  5. Run `sworn top test-release /path/to/project` again.
  6. Observe the green-board output showing all journeys validated.

## Delivered

- **AC 1: Green-board when all pass** — `TestTop_GreenBoard` verifies that sworn top exits 0 and renders "Green-board ✓" when all journeys have walked-pass attestations. Evidence: `cmd/sworn/top_test.go` lines 22-57.
- **AC 2: Kill-list when any un-walked or failed** — `TestTop_KillList_Unwalked` (exit 1, kill-list naming the journey) and `TestTop_KillList_Failed` (exit 1, kill-list labelling walked-fail). Evidence: `cmd/sworn/top_test.go` lines 59-131.
- **AC 3: Strictly read-only** — `TestTop_ReadOnly` snapshots the filesystem before and after running sworn top and asserts no files were created or modified. Evidence: `cmd/sworn/top_test.go` lines 133-168.
- **AC 4: Empty state hint when no journeys artefact** — `TestTop_EmptyState` verifies exit 0 and output hinting to run `sworn journeys`. Evidence: `cmd/sworn/top_test.go` lines 9-20.
- **Integration (Rule 1): cmdTop entry point** — `TestTopCmd_MixedStatuses` calls the actual command entry point `cmdTop([]string{"test-release", dir})`, not `renderEvidenceSurface`, and verifies exit 1 for a mixed-status kill-list. Evidence: `cmd/sworn/top_test.go`.

## Not delivered

None. All four acceptance checks are delivered. Both verifier violations from the initial FAIL round are resolved.

## Divergence from plan

The planned touchpoints (`planned_files`) listed only `cmd/sworn/top.go`, `cmd/sworn/top_test.go`, and `cmd/sworn/main.go`. Two additional files were needed:

- `internal/journey/walkthrough.go` — the walkthrough attestation model and loading API. The spec describes sworn top as reading "journey attestations (`internal/journey`) via their existing public APIs." Since no attestation API existed in the journey package (S13 is planned), this slice adds the types and API that sworn top consumes. This is a natural forward-extension of the journey package — S13 will populate the attestation artefact; S15 reads it.
- `internal/journey/walkthrough_test.go` — tests for the new walkthrough API.

This divergence is recorded and is consistent with the spec's risk section: "it can be built against empty/fixture attestation data and renders the empty state cleanly until S13 is live."

## Verifier violations resolved (re-entry from `failed_verification`)

This session re-entered from `failed_verification` state. Two violations were addressed:

1. **Gate 2 — start_commit**: Changed from `a58733d` (the implementation commit) to `e3b0ec2` (the materialise-commit immediately before implementation). The corrected `git diff --name-only e3b0ec2..HEAD` now shows all 9 changed files including the planned touchpoints.
2. **Gate 3 — cmdTop integration**: Added `TestTopCmd_MixedStatuses` which calls `cmdTop([]string{"test-release", dir})`, the command entry point, not `renderEvidenceSurface`. Follows the same pattern as `lint_ac_test.go`.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S15-sworn-top-evidence 2026-06-16-fidelity-layer
release-verify.sh
  slice:       S15-sworn-top-evidence
  slice dir:   docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence
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
  PASS  9 file(s) changed vs main
  (first 20)
    cmd/sworn/main.go
    cmd/sworn/top.go
    cmd/sworn/top_test.go
    docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/journal.md
    docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/proof.md
    docs/release/2026-06-16-fidelity-layer/S15-sworn-top-evidence/status.json
    docs/release/2026-06-16-fidelity-layer/index.md
    internal/journey/walkthrough.go
    internal/journey/walkthrough_test.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

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
  checks passed: 18
  checks failed: 0

FIRST-PASS PASS
Open a FRESH session and paste role-prompts/verifier.md to perform adversarial verification.
Do NOT run the verifier in this same session -- Rule 7 requires a fresh context window.
```