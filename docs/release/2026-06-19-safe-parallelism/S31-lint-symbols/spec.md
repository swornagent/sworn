---
title: 'S31-lint-symbols — `sworn lint symbols` greps design identifiers against the live codebase'
description: 'Theme T-C (~25 rows): a design names a function, field, constant, or table that does not exist or is the wrong one — a guaranteed compile error or empty query if shipped. Add a `sworn lint symbols` subcommand that extracts back-ticked identifiers from a slice''s design and greps them against the live codebase; an unresolved identifier emits an advisory warning (non-zero advisory). Harvested from the trial-log analysis §3a #3 ("grep the symbol").'
---

# Slice: `S31-lint-symbols`

## User outcome

A planner or implementer running `sworn lint symbols <slice-id> <release>` is warned
**before code** about every back-ticked identifier in the slice's design — function,
field, constant, table — that cannot be resolved against the live codebase. This
mechanises the "grep the symbol" lesson: theme T-C is almost entirely preventable
because designs *infer* a symbol name instead of grepping for it, producing a compile
error or an empty query once shipped.

## Entry point

`sworn lint symbols <slice-id> <release>` (CLI). Verifiable by: an integration-style
test with a temp design referencing both a symbol that exists in a fixture tree and one
that does not; assert the unresolved symbol is reported and the resolved one is not.

## In scope

- New `sworn lint symbols` target dispatched from `cmd/sworn/lint.go`.
- A `internal/lint` helper (`symbols.go`) that:
  - extracts back-ticked identifiers from the slice's design that look like code
    symbols (CamelCase, snake_case, `Type.Field`, table-like names);
  - greps them against the live codebase (the repo root, excluding the design docs);
  - emits an **advisory warning** for each unresolved identifier.
- Advisory severity: the command surfaces unresolved symbols and returns a non-zero
  **advisory** code (distinct from the hard fail-closed of S29/S30) so it can run as a
  warn-only gate without blocking — per the harvest's "unresolved → warn".

## Out of scope

- File/package touchpoint reconciliation (S30) and dependency reconciliation (S29).
- Resolving symbols semantically (type-checking) — a textual grep is sufficient and
  matches the harvest recommendation; a symbol present anywhere in the tree resolves.
- Hard fail-closed: this gate is advisory by design (it cannot distinguish a symbol the
  slice *introduces* from a typo), so it warns rather than blocks.

## Planned touchpoints

- `internal/lint/symbols.go` (new)
- `internal/lint/symbols_test.go` (new)
- `cmd/sworn/lint.go` (extend the target switch with `symbols`)

> **Touchpoint note:** `internal/lint` is the shared package introduced by S29; this
> slice adds `symbols.go`. Serialised within track T12 with S29/S30 — no parallel
> `internal/lint` collision. The "grep against the live codebase" surface is the repo
> working tree (same root the existing `cmd/sworn/lint.go` targets resolve from).

## Acceptance checks

- [ ] `sworn lint symbols <slice> <release>` reports each back-ticked identifier in the
  design that cannot be grep-resolved against the codebase, naming the identifier
- [ ] `sworn lint symbols <slice> <release>` does **not** report identifiers that
  resolve (present somewhere in the live tree)
- [ ] the command returns a non-zero **advisory** code when there is at least one
  unresolved identifier, and exit 0 when all resolve
- [ ] `go build ./...` and `go vet ./internal/lint/...` pass

## Required tests

- **Unit / integration** `internal/lint/symbols_test.go`:
  - `TestSymbolsUnresolvedWarns`: design references `CalculateFIRE` absent from the
    fixture tree → reported, advisory non-zero
  - `TestSymbolsResolvedQuiet`: design references a symbol present in the fixture tree
    → not reported
  - `TestSymbolsAllResolvedExitZero`: every referenced symbol resolves → exit 0
- **Reachability artefact**: run `sworn lint symbols` against a fixture release with an
  unresolved identifier; capture the advisory output + non-zero exit. Document in `proof.md`.

## Risks

- Over-extraction (back-ticked prose that is not a symbol — e.g. a CLI flag) yields
  noisy false warnings. Mitigation: restrict extraction to identifiers matching a
  code-symbol shape (CamelCase / snake_case / dotted / table-like) and keep severity
  advisory so a false positive never blocks. Evidence the class is real:
  `S04b-tui-live` (`started_at` in the wrong table), `S05-drift-api`
  (`LoadEnvelopeByID` did not exist), `S16` (`Calculate` vs `CalculateFIRE`).

## Deferrals allowed?

None.
