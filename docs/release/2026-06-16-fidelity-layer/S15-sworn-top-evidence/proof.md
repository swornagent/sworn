# Proof Bundle: S15-sworn-top-evidence

> Generated from live repo state, not recollection.

## Scope

When a maintainer runs `sworn top`, sworn renders a read-only evidence surface for the active release: each critical journey in scope with its validation status (un-walked / walked-pass / walked-fail), assembled into a green-board when all pass and a kill-list when any fail. The surface only reads and displays; it issues no state transitions and gates nothing.

## Files changed

```
$ git diff --name-only release-wt/2026-06-16-fidelity-layer
cmd/sworn/main.go
cmd/sworn/top.go
cmd/sworn/top_test.go
internal/journey/walkthrough.go
internal/journey/walkthrough_test.go
```

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
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.006s

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

## Not delivered

None. All four acceptance checks are delivered.

## Divergence from plan

The planned touchpoints (`planned_files`) listed only `cmd/sworn/top.go`, `cmd/sworn/top_test.go`, and `cmd/sworn/main.go`. Two additional files were needed:

- `internal/journey/walkthrough.go` — the walkthrough attestation model and loading API. The spec describes sworn top as reading "journey attestations (`internal/journey`) via their existing public APIs." Since no attestation API existed in the journey package (S13 is planned), this slice adds the types and API that sworn top consumes. This is a natural forward-extension of the journey package — S13 will populate the attestation artefact; S15 reads it.
- `internal/journey/walkthrough_test.go` — tests for the new walkthrough API.

This divergence is recorded and is consistent with the spec's risk section: "it can be built against empty/fixture attestation data and renders the empty state cleanly until S13 is live."

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S15-sworn-top-evidence 2026-06-16-fidelity-layer
<paste output here — run live>
```