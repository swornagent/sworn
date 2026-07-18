---
title: 'Proof bundle — S01-tui-bounded-navigation'
description: 'Bounded catalog loading and terminal-height-safe TUI navigation.'
date: 2026-07-18
---

# Proof bundle — S01-tui-bounded-navigation

> Rendered from `proof.json` (proof-v1). The JSON record is the source of truth.

## Scope

A SwornAgent operator starts with the newest ten releases, loads older releases in bounded
ten-record transactions, keeps both pane selections reachable inside the terminal height across
resize, and moves between focused panes with Right/Left or Enter/Esc.

## Files changed

`git diff --name-only e2e445f0c63e2cf6408755faf259419b5ed621a6` identifies the shared board
discovery authority, the root TUI and pane implementation/tests, this slice's status/journal/proof,
and the three Captain-required terminal frames listed in `proof.json`.

## Test results

- Coverage passed with 5/5 ACs mapped to tests; mock lint found no undeclared boundary.
- Required board/TUI/CLI package tests and the repository-wide `go test ./...` suite passed.
- `go vet ./...` passed and `gofmt -l .` produced no output.
- The configured AC-satisfaction check returned `PASS` with no findings.
- The Implementer maintainability preflight returned `PASS` for the canonical semantic scope at
  `737fb77b3a9a7aba294127a24ec3f7deee11d8a0`; its immutable report and fingerprint are pinned in
  `status.json`.

## Reachability artefact

Root Bubble Tea `Model.Update` and `Model.View` journeys drive bounded startup, asynchronous older
loading, serial refresh delivery, cursor-relative release and board scrolling, resize, focus and
arrow transitions, and every positive-height bound. `TestBoundedNavigationTerminalFrames`
byte-compares normal and constrained frames under `screenshots/S01-tui-bounded-navigation/`,
covering both focus states and `o older`, `loading older`, and `all releases loaded`.

## Delivered

- **AC-01:** one board-owned core performs bounded and complete ranking, validation, topology
  election, and status election; excluded object records are not read or parsed.
- **AC-02:** startup primes ten newest releases and `o` asynchronously grows the window by ten with
  exact footer states and selection preservation.
- **AC-03:** generation-plus-limit identity and one in-flight owner prevent overlap and stale smaller
  replacement while preserving desired depth and the last good snapshot on failure.
- **AC-04:** independent cursor-relative windows keep every release and slice reachable across
  resize, track boundaries, and tiny positive heights without exceeding the terminal frame.
- **AC-05:** Right/Enter and Left/Esc share transitions; focus controls accent borders and exact help;
  board `o` retains the existing order toggle.

## Not delivered

None.

## Divergence from plan

- Captain PIN-3 added the three proof-only terminal frames outside the production touchpoint list.
- `internal/tui/styles.go` required no edit because focus colour selection is root-model state with no
  geometry change.

## First-pass verdict

`PASS` — the proof-bundle gate exited 0 against this live bundle with `cost_usd: 0`.
