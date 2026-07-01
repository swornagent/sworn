---
title: 'S26 — sworn telemetry: per-model eval projections'
description: 'sworn telemetry reports per-model rework rate, mean tokens-per-turn, mean latency_ms, and estimated cost; output is machine-readable JSON (--json flag) and human-readable table (default).'
---

# Slice: `S26-eval-projections`

## User outcome

`sworn telemetry --release <name>` (or `sworn telemetry report`) outputs a per-model summary table showing: model ID, dispatch count, rework rate (re-verify attempts / total dispatches), mean tokens/turn, mean duration_ms, and estimated cost. A `--json` flag produces machine-readable output for downstream tooling.

## Entry point

`cmd/sworn/telemetry.go` (existing file, currently has events subcommand); add `report` subcommand or use `sworn telemetry` with no subcommand as the entry point.

## In scope

- `cmd/sworn/telemetry.go`: add `sworn telemetry [--release <name>] [--json]` report mode that:
  1. Opens `.sworn/supervisor-<name>.db` (from S25)
  2. Queries the `events` and `decisions` tables for all dispatches in the release
  3. Groups by `model_id_confirmed` (from S24's enriched Dispatch records)
  4. Computes per-model: `dispatch_count`, `rework_rate = (dispatches with attempt > 0) / total`, `mean_input_tokens`, `mean_output_tokens`, `mean_duration_ms`, `total_cost_usd`
  5. Outputs as a human-readable table (default) or JSON (`--json`)
- The data source is the `dispatches` JSON embedded in status.json files (or the durable DB if the events table stores dispatch-level data); prefer reading from status.json files across the release (authoritative per-slice ground truth)
- If a model has no `duration_ms` or `input_tokens` (older runs before S24), those columns show "—" in the table and null in JSON

## Out of scope

- Cross-release aggregation
- Regression detection or alerting
- The eval "projections" are computed, not ML-based (simple arithmetic aggregations)

## Planned touchpoints

- `cmd/sworn/telemetry.go` (add report subcommand/mode)

## Acceptance checks

- [ ] `sworn telemetry --release <name>` compiles and exits 0 when the release has at least one status.json with a non-empty dispatches array
- [ ] The output table includes columns: Model, Dispatches, Rework Rate, Mean Input Tokens, Mean Output Tokens, Mean Duration (ms), Total Cost (USD)
- [ ] WHEN `--json` flag is passed, THE SYSTEM SHALL output a JSON array with one object per model ID containing the same fields
- [ ] WHEN a dispatch has no `duration_ms` (value is 0), THE SYSTEM SHALL exclude it from the mean_duration_ms computation (not skew the mean with 0s from pre-S24 dispatches)
- [ ] `telemetry_test.go`: test with a mock release containing two slices, each with one dispatch — verify output contains correct rework rate and mean tokens

## Required tests

- **Unit**: `cmd/sworn/telemetry_test.go` (new or extend) — mock status.json files; verify report output correctness
- **Reachability artefact**: `sworn telemetry --release 2026-06-27-conformance-foundation --json` exits 0 with valid JSON output on a real run

## Risks

- Reading from status.json files across the release requires iterating all slices; the release oracle (board.Oracle or direct filesystem read) can provide the file list

## Deferrals allowed?

No.
