---
title: 'S53-ledger-cli — sworn ledger sync + report over the verdict corpus'
description: 'Adds the `sworn ledger` command (sync + report) on top of S52''s projection engine. sync scans every release board into docs/ledger/verdicts.jsonl; report aggregates pass-rate by model x slice_kind, attempts-to-pass, and a gate-failure histogram. Registers via a per-file init() to avoid the cmd/sworn/commands.go shared-file collision.'
---

# Slice: `S53-ledger-cli`

## User outcome

A maintainer runs `sworn ledger sync` and the whole board's verdict history is harvested
into `docs/ledger/verdicts.jsonl`; then runs `sworn ledger report` and sees, in the
terminal, the pass-rate of each implementer model broken down by slice kind, the
distribution of attempts-to-pass, and which acceptance-gate categories fail most often. The
"strategic IP" — accumulated judgement about what works in this repo — becomes legible
instead of trapped in scattered `status.json` files.

## Entry point

`sworn ledger sync` and `sworn ledger report` — a new subcommand registered in the S51
command registry from a per-file `init()` in `cmd/sworn/ledger.go`. `sworn ledger` with no
subcommand prints usage listing `sync` and `report`.

## Background

S52 ships `internal/ledger` (the `Record` schema, `Project`, `Append`, idempotent `Key`).
This slice is the user-reachable surface over it: a command that walks the board and a
report that aggregates the corpus. Registration uses a per-file `init()` calling
`command.Register` (the S51/T15 pattern) rather than editing the central
`cmd/sworn/commands.go` — the latter is the shared file whose collisions the registry was
built to remove (see the 2026-06-22 `main.go` touchpoint-collision capture).

## In scope

- New `cmd/sworn/ledger.go`:
  - `func init()` registers `command.Command{Name:"ledger", Summary:..., Run:runLedger}`.
  - `runLedger(args)` dispatches `sync` and `report`; no/unknown subcommand prints usage and
    returns non-zero per the fail-closed convention.
  - `sync`: discovers every `docs/release/*/*/status.json`, reads the matching `spec.md` to
    count `- [ ]` gates, calls `ledger.Project` + `ledger.Append` into
    `docs/ledger/verdicts.jsonl`; prints how many records were added vs already present.
- New `internal/ledger/query.go`:
  - `Load(path string) ([]Record, error)` — reads the JSONL corpus.
  - `PassRateByModelKind(records) ...` — pass-rate per (model, slice_kind) with counts.
  - `AttemptsToPass(records) ...` — distribution of `attempt` at the passing verdict.
  - `GateFailureHistogram(records) ...` — frequency of failing gate categories across FAILs.
  - A `Report` renderer producing a plain-text table (no new deps; `text/tabwriter`).
- `sworn ledger report` reads the corpus and prints the three aggregates.

## Out of scope

- The projection engine, schema, and model/attempt capture — **S52** (consumed here).
- Routing recommendations and the config-resolver wire — **S54** (adds `recommend` to this
  same `cmd/sworn/ledger.go`).
- Any network call or remote upload — the corpus is local + git-tracked only.
- A TUI surface for the ledger — not in this release. Why: the CLI report is the MVP harvest
  surface; a TUI panel is additive polish. Tracking: future-release ledger follow-up. Ack:
  Brad, 2026-06-22.

## Planned touchpoints

- `cmd/sworn/ledger.go` (new — init-registers `ledger`; `sync` + `report` dispatch)
- `cmd/sworn/ledger_test.go` (new)
- `internal/ledger/query.go` (new — `Load` + aggregations + report renderer)
- `internal/ledger/query_test.go` (new)

## Acceptance checks

- [ ] `sworn ledger` with no subcommand prints usage naming `sync` and `report` and returns
  a non-zero exit code (fail-closed)
- [ ] `sworn ledger sync` over a fixture release tree appends one line per verified slice to
  `docs/ledger/verdicts.jsonl` and reports the added/skipped counts; a second run adds zero
  (idempotent, via S52's `Key`)
- [ ] `sworn ledger sync` counts each slice's gates by reading `- [ ]` lines from its
  `spec.md`, and the resulting records carry the correct `gate_count`
- [ ] `sworn ledger report` over a fixture corpus prints a pass-rate table grouped by model
  and slice_kind, an attempts-to-pass distribution, and a gate-failure histogram
- [ ] `command.Lookup("ledger")` returns the registered command (proves the `init()` wired
  into the S51 registry — the Rule 1 integration point, not a leaf-only test)
- [ ] `go test ./internal/ledger/... ./cmd/sworn/...` passes; `go build ./...` succeeds with
  no new `go.mod` deps

## Required tests

- **Unit**: `internal/ledger/query_test.go` — aggregations over a fixed in-memory corpus
  with known expected pass-rates / histogram buckets.
- **Integration**: `cmd/sworn/ledger_test.go` — `sync` against a temp fixture release tree
  asserting the JSONL output and idempotency; a `command.Lookup("ledger")` assertion proving
  registry reachability.
- **Reachability artefact**: in `proof.md`, paste real `sworn ledger sync` then
  `sworn ledger report` terminal output run against the actual repo board.

## Risks

- `sync` discovering `status.json` across releases must not choke on in-progress boards with
  partial/empty `verification` blocks — `Project` already returns `ok==false` for those;
  ensure `sync` skips rather than errors.
- Gate counting reads `spec.md`; a spec using a non-`- [ ]` AC style would under-count.
  Acceptable for this repo (the template mandates `- [ ]`); note the assumption in code.

## Deferrals allowed?

Yes, with Rule 2 compliance — the TUI surface is surfaced above with why / tracking / ack.
