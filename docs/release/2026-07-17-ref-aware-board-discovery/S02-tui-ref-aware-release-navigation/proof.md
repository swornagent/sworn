---
title: 'Proof bundle — S02-tui-ref-aware-release-navigation'
description: 'Ref-aware TUI release navigation with immutable catalog snapshots and explicit uncommitted evidence.'
date: 2026-07-18
---

# Proof bundle — S02-tui-ref-aware-release-navigation

> Rendered from `proof.json` (proof-v1). The JSON record is the source of truth.

## Scope

The SwornAgent TUI discovers and asynchronously opens the same immutable ref-only release catalog
snapshot and high-water slice evidence as the board command. It marks uncommitted evidence in text
and consumes the board package's read-only filesystem fallback when no usable Git HEAD exists.

## Files changed

`git diff --name-only 406430bc639421dd4319b60aee5f04a2d505c3cb..HEAD` identifies the
implementation surfaces in `internal/board/discovery*` and `internal/tui/{board,model,releases,tui_test}.go`,
plus this slice's proof/status/journal and the planner's committed re-scope artefacts.

## Test results

- Required package tests passed: `go test ./internal/tui ./internal/board ./cmd/sworn`.
- Repository-wide tests passed: `go test ./...`.
- Static checks passed: `go vet ./...`; `gofmt -l .` produced no output.
- Coverage passed with 4/4 ACs mapped to tests.
- The configured AC-satisfaction check returned `PASS` with four non-blocking informational findings.

## Reachability artefact

The verbose named test command recorded in `proof.json` passed all nine integration cases. The
primary fixture creates a real temporary Git repository whose HEAD contains no release board,
selects `refs/heads/release-wt/ref-only-release`, drives Enter through the root Bubble Tea
`Model.Update`, executes the returned asynchronous command, and observes the same T1 plan and
verified/committed SliceState evidence as `board.DiscoverCatalog` (the `sworn board` authority).

## Delivered

- **AC-01:** one selected CatalogRecord supplies release identity, topology, and high-water state.
- **AC-02:** exact `[uncommitted]` text is derived only from catalog durability evidence.
- **AC-03:** no-HEAD filesystem discovery is read-only, ordered, provenance-conservative, and
  fail-closed for a present malformed or identity-mismatched board.
- **AC-04:** ordering, navigation, Loading… feedback, async completion, and sourceRef staleness
  protection remain intact.

## Not delivered

None.

## Divergence from plan

None.

## First-pass verdict

`PASS` — the proof-bundle gate exited 0 against this live bundle.
