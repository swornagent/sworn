---
title: Slice proof bundle — S02-tui-ref-aware-release-navigation
description: Rule 6 proof bundle rendered from proof.json and live repository checks.
---

# Proof Bundle: `S02-tui-ref-aware-release-navigation`

Rendered from `proof.json` (proof-v1). First passing implementation proof.

## Scope

The SwornAgent TUI discovers and opens non-HEAD release boards from the shared
catalog snapshot, preserves selected `sourceRef` identity across asynchronous
loads, and visibly marks uncommitted state evidence.

## Test results

- `go test ./internal/tui ./internal/board ./cmd/sworn` — PASS
- `go test ./...` — PASS
- `go vet ./...` — PASS
- `gofmt -l .` — PASS (empty output)
- semantic-coverage check — PASS
- AC-satisfaction check — PASS for AC-01 through AC-04

## Reachability artefact

Integration fixtures exercise ref-only discovery, CLI/TUI state-evidence parity,
catalog-backed asynchronous board loading, stale-`sourceRef` rejection, shared
non-Git fallback, and committed versus uncommitted textual rendering.

## Delivered

- AC-01: ref-only releases and their elected state evidence reach the TUI from
  the selected catalog record.
- AC-02: uncommitted evidence receives textual markers in the board and selected
  release aggregate; committed evidence does not.
- AC-03: catalog errors remain fail-closed, while no-HEAD filesystem fallback is
  read-only, ordered, and conservatively classified as uncommitted.
- AC-04: existing navigation remains bounded and asynchronous results are
  accepted only when both release ID and `sourceRef` still match.

## Not delivered

None.

## Divergence from plan

None.
