---
title: Proof Bundle S11-journey-elicitation
description: Rule 6 proof bundle for the journey model, CLI, and gate.
---

# Proof Bundle: S11-journey-elicitation

## Scope

When a maintainer runs `sworn journeys <project>`, sworn presents AI-drafted critical customer journeys inferred from the app, the human ratifies/adjusts them, and the result is written to a durable, version-controlled journeys artefact. `sworn journeys --check` fails closed if the artefact is missing or unratified.

## Files changed

```
$ git diff --name-only 0535a74..HEAD
cmd/sworn/journeys.go
cmd/sworn/journeys_test.go
cmd/sworn/main.go
docs/release/2026-06-16-fidelity-layer/S11-journey-elicitation/status.json
internal/adopt/adopt.go
internal/adopt/baton/VERSION
internal/adopt/baton/rules/10-customer-journey-validation.md
internal/journey/journey.go
internal/journey/journey_test.go
internal/prompt/planner.md
```

## Test results

### Go (journey package)

```
$ go test ./internal/journey/... -v
=== RUN   TestCheck_MissingArtefact
--- PASS: TestCheck_MissingArtefact (0.00s)
=== RUN   TestCheck_UnratifiedArtefact
--- PASS: TestCheck_UnratifiedArtefact (0.00s)
=== RUN   TestCheck_RatifiedArtefact
--- PASS: TestCheck_RatifiedArtefact (0.00s)
=== RUN   TestListJourneys
--- PASS: TestListJourneys (0.00s)
=== RUN   TestListJourneys_NilArtefact
--- PASS: TestListJourneys_NilArtefact (0.00s)
=== RUN   TestListJourneys_EmptyArtefact
--- PASS: TestListJourneys_EmptyArtefact (0.00s)
=== RUN   TestDraftTemplate
--- PASS: TestDraftTemplate (0.00s)
=== RUN   TestRatify_EmptyArtefact
--- PASS: TestRatify_EmptyArtefact (0.00s)
=== RUN   TestRatify_MissingName
--- PASS: TestRatify_MissingName (0.00s)
=== RUN   TestRatify_Success
--- PASS: TestRatify_Success (0.00s)
=== RUN   TestAddJourney_InvalidatesRatification
--- PASS: TestAddJourney_InvalidatesRatification (0.00s)
=== RUN   TestSaveAndLoadArtefact
--- PASS: TestSaveAndLoadArtefact (0.00s)
=== RUN   TestLoadArtefact_NotExist
--- PASS: TestLoadArtefact_NotExist (0.00s)
=== RUN   TestJourneyArtefactPath
--- PASS: TestJourneyArtefactPath (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/journey	0.004s
```

### Go (journeys CLI command)

```
$ go test ./cmd/sworn/ -run TestJourneys -v
=== RUN   TestJourneysCmd_MissingCheck
--- PASS: TestJourneysCmd_MissingCheck (0.00s)
=== RUN   TestJourneysCmd_UnratifiedCheck
--- PASS: TestJourneysCmd_UnratifiedCheck (0.00s)
=== RUN   TestJourneysCmd_PassCheck
--- PASS: TestJourneysCmd_PassCheck (0.00s)
=== RUN   TestJourneysCmd_Elicit
--- PASS: TestJourneysCmd_Elicit (0.00s)
=== RUN   TestJourneysCmd_ElicitWithExistingArtefact
--- PASS: TestJourneysCmd_ElicitWithExistingArtefact (0.00s)
=== RUN   TestJourneysCmd_PassPrint
--- PASS: TestJourneysCmd_PassPrint (0.00s)
=== RUN   TestJourneysCmd_NoArgs
--- PASS: TestJourneysCmd_NoArgs (0.00s)
=== RUN   TestJourneysCmd_NonExistentPath
--- PASS: TestJourneysCmd_NonExistentPath (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.007s
```

### Full suite

```
$ go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn	0.029s
ok  	github.com/swornagent/sworn/internal/adopt	(cached)
... (all pass)
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: N/A (CLI tool, no screenshot)
- **User gesture**:
  ```
  $ sworn journeys --check /tmp/test-project
  FAIL: no journeys artefact found at /tmp/test-project/.sworn/journeys.json.
  Elicitation has not been run. Run 'sworn journeys <project>' to start.
  $ sworn journeys /tmp/test-project
  Journeys artefact drafted at /tmp/test-project/.sworn/journeys.json.
  Draft journeys:
     J-develop-feature ...
     J-initial-setup ...
  $ # (edit .sworn/journeys.json, set is_ratified=true, ratified_by="me", ratified_at=...)
  $ sworn journeys --check /tmp/test-project
  Journeys artefact found and ratified by me.
     J-develop-feature: developer — ...
  ```

## Delivered

- **[AC1] WHEN no journeys artefact exists for a project, THE SYSTEM SHALL exit non-zero from `sworn journeys --check` and state that elicitation has not been run.** — evidence: `TestCheck_MissingArtefact`, `TestJourneysCmd_MissingCheck`.
- **[AC2] WHEN a journeys artefact exists but is unratified by a human, THE SYSTEM SHALL fail and name it as unratified.** — evidence: `TestCheck_UnratifiedArtefact`, `TestJourneysCmd_UnratifiedCheck`.
- **[AC3] WHEN `sworn journeys <project>` runs, THE SYSTEM SHALL draft >=1 candidate journey from the app and present it for human ratification.** — evidence: `TestDraftTemplate`, `TestJourneysCmd_Elicit`. The draft scans the project's `cmd/` and `internal/` directories to infer journeys; the artefact is saved unratified.
- **[AC4] WHEN the artefact exists and is human-ratified, THE SYSTEM SHALL exit 0 from `sworn journeys --check` and list the journeys.** — evidence: `TestCheck_RatifiedArtefact`, `TestJourneysCmd_PassCheck`, `TestJourneysCmd_PassPrint`.
- **[AC5] THE SYSTEM SHALL persist ratified journeys to a version-controlled file so they survive session boundaries.** — evidence: `TestSaveAndLoadArtefact` (round-trip save/load of `.sworn/journeys.json`). The file is JSON in the project's `.sworn/` directory, designed to be committed to version control.

## Not delivered

- **Model-assisted draft** (the AI reads the app and infers richer journeys): **Why**: Provisional — the schema and draft strategy are refined via the live journey-validation hand-run. **Tracking**: Provisional schema field acknowledged 2026-06-16 in status.json open_deferrals. **Refinement**: Via `/replan-release` post hand-run.

## Divergence from plan

- The `internal/prompt/planner.md` was updated with journey elicitation guidance as a new section inserted before "Working style notes", rather than a standalone prompt file — this keeps the elicitation guidance co-located with the existing planner prompt.
- The draft template (`DraftTemplate`) scans the project's file system to produce candidate journeys rather than using an AI model — the model-assisted draft is deferred as provisional per the spec's own acknowledgement.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S11-journey-elicitation 2026-06-16-fidelity-layer
release-verify.sh
  slice:       S11-journey-elicitation
  slice dir:   docs/release/2026-06-16-fidelity-layer/S11-journey-elicitation
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
  PASS  state is 'implemented' — ready for verifier

== Diff vs main ==
  PASS  21 file(s) changed vs main

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

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: 18
  checks failed: 0

FIRST-PASS PASS
```