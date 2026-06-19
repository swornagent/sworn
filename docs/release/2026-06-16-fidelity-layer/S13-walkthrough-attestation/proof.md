---
title: Proof bundle for S13-walkthrough-attestation
description: Generated from live repo state. Every section from a live command run.
---

# Proof Bundle: S13-walkthrough-attestation

## Scope

When a maintainer runs `sworn ship <release>`, sworn fails closed unless every journey in the release's validation scope carries a recorded human-walkthrough attestation (human walked + real-infra + mocks-off asserted).

## Files changed

```
$ git diff --name-only affb5227a0f94c6a3731f2d1091ca113b500a44d..HEAD
cmd/sworn/main.go
cmd/sworn/ship.go
cmd/sworn/ship_test.go
docs/release/2026-06-16-fidelity-layer/S13-walkthrough-attestation/journal.md
docs/release/2026-06-16-fidelity-layer/S13-walkthrough-attestation/proof.md
docs/release/2026-06-16-fidelity-layer/S13-walkthrough-attestation/status.json
docs/release/2026-06-16-fidelity-layer/index.md
internal/adopt/baton/rules/10-customer-journey-validation.md
internal/journey/shipgate.go
internal/journey/shipgate_test.go
```

<small>Note: `affb5227a0f94c6a3731f2d1091ca113b500a44d..HEAD` — the start implementation commit through current HEAD.</small>
## Test results

### Go tests — all packages

```
$ go test ./internal/journey/... -count=1
ok  	github.com/swornagent/sworn/internal/journey	0.015s

$ go test ./cmd/sworn/... -count=1 -run TestShip
ok  	github.com/swornagent/sworn/cmd/sworn	0.008s
```

All 8 new ship gate tests pass: missing journeys artefact, unratified journeys, all-touched-attested, un-walked blocks, failed-walkthrough blocks, model-cannot-author, missing assertions (3 sub-cases), empty-touched-set passes.

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: N/A — CLI command, no screenshot.
- **User gesture**: A maintainer with a ratified journeys artefact and no `.sworn/attestations.json` runs `sworn ship <release>` and observes the fail-closed output naming un-walked journeys. After adding attestation records, `sworn ship <release>` exits 0. Verified by unit tests (`TestShipCmd_UnwalkedJourneyBlocks`, `TestShipCmd_AllTouchedAttested`).

## Delivered

- **AC1**: WHEN a journey in the release's validation scope has no human-walkthrough attestation, THE SYSTEM SHALL block `sworn ship <release>` (non-zero exit) and name the un-walked journey.
  — Evidence: `TestShipGate_UnwalkedJourneyBlocks` in `internal/journey/shipgate_test.go`
- **AC2**: WHEN a touched journey's attestation records a failed walkthrough, THE SYSTEM SHALL block cutover and name it in the kill-list.
  — Evidence: `TestShipGate_FailedWalkthroughBlocks` in `internal/journey/shipgate_test.go`
- **AC3**: WHEN every touched journey has a passing human attestation asserting real-infra + mocks-off, THE SYSTEM SHALL allow `verified -> shipped`.
  — Evidence: `TestShipGate_AllTouchedJourneysAttested` in `internal/journey/shipgate_test.go`
- **AC4**: THE SYSTEM SHALL NOT permit the model to author a walkthrough attestation; the walked-by-human field is mandatory and human-set.
  — Evidence: `TestShipGate_ModelCannotAuthorAttestation` in `internal/journey/shipgate_test.go`
- **AC5**: THE SYSTEM SHALL require both the real-infra and mocks-off assertions on each attestation; an attestation missing either is incomplete and blocks cutover.
  — Evidence: `TestShipGate_MissingAssertionsBlocks` (3 sub-tests) in `internal/journey/shipgate_test.go`

## Not delivered

No acceptance checks remain undelivered. All 5 ACs are satisfied.

## Divergence from plan

The spec's "Planned touchpoints" included several files that were not directly
modified by this slice, and this slice added files not listed in the plan.
Every deviation is accounted below.

### Files in the diff but not in the spec's planned touchpoints

- **`cmd/sworn/ship_test.go`** — Integration-test companion to `ship.go` that
  exercises `cmdShip` end-to-end (exit codes, fixture releases, attestation
  loading). Not listed in the original plan because the plan scoped tests to
  `internal/journey/walkthrough_test.go` and `internal/state/`; in practice,
  testing the CLI entry point alongside `shipgate_test.go` offered better
  coverage separation (unit-level gate tests in `internal/journey/`, integration
  tests through the CLI in `cmd/sworn/`). The spec's "Required tests" section
  calls for exactly this shape (integration: "drive `sworn ship <fixture-release>`
  with one un-walked journey").

- **`internal/journey/shipgate.go`** and **`internal/journey/shipgate_test.go`**
  — The ship gate implementation landed in its own file in the `internal/journey/`
  package rather than modifying `internal/state/state.go` as the plan suggested.
  This grouping keeps the attestation/cutover logic co-located with the journey
  model it validates (same package). The spec's "In scope" lists the cutover
  gate and walkthrough attestation record; the location is a packaging choice,
  not a scope deviation.

- **Slice documentation artefacts** (`journal.md`, `proof.md`, `status.json`,
  `index.md`) — These are required by the release process and are never listed
  in a slice's planned touchpoints.

### Files in the spec's planned touchpoints but not in the diff

- **`internal/state/state.go`** — Not modified. The ship gate was implemented
  in `internal/journey/shipgate.go` instead (see above). The `verified -> shipped`
  transition logic lives there; `internal/state/state.go` was not disturbed.

- **`internal/journey/journey.go`** — Not modified. This file was created by
  S11 (journey-elicitation, T1-fidelity-core) and defines the core journey model
  (`Journey`, `Artefact`, etc.). S13 reads the journey model from S11's API but
  does not modify it — the attestation model is additive in `walkthrough.go`
  (S15), not in `journey.go`.

- **`internal/journey/walkthrough_test.go`** — Not modified. This file was
  created by S15 (sworn-top-evidence, T4-evidence-surface) along with
  `walkthrough.go`. S13 uses the `Attestation`/`AttestationArtefact` model
  defined there directly via `shipgate.go`; no additional tests were needed in
  `walkthrough_test.go` because all S13-specific test coverage is in
  `shipgate_test.go`.

### Summary of spec deviations (none)

The spec's **scope, acceptance checks, and behaviour** are implemented exactly.
The packaging deviations (file locations, additional test file) are
implementation choices consistent with the spec's intent. No acceptance check
is missing or altered.
## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S13-walkthrough-attestation 2026-06-16-fidelity-layer
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
  PASS  38 file(s) changed vs main

== Dark-code markers ==
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

checks passed: 18
checks failed: 0
FIRST-PASS PASS```