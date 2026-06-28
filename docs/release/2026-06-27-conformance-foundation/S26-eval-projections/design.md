# Design TL;DR — S26-eval-projections

## Approach

Add a `report` subcommand to the existing `sworn telemetry` CLI surface (`cmd/sworn/telemetry.go`). The command walks every slice's `status.json` under `docs/release/<name>/*/`, reads `verification.dispatches[]`, groups by model, computes per-model aggregates, and outputs either a human-readable table (default) or JSON (`--json`).

## Key design choices

### 1. Data source: status.json files (not the supervisor DB)

**Rationale:** The spec states a preference for status.json as the "authoritative per-slice ground truth." The supervisor event store (`supervisor-<name>.db`) records process-ownership events — not dispatch-level token/duration/cost data. Dispatch records live exclusively in `status.json → verification.dispatches[]`, enriched by S24 with `duration_ms`, `input_tokens`, `output_tokens`, `model_id_confirmed`, and `cost_usd`. Reading from status.json avoids a new DB schema dependency and reuses the same glob-walk pattern already established by `sworn ledger sync` (`ledger.go §50-68`).

### 2. Subcommand: `sworn telemetry report`

**Rationale:** The existing `cmdTelemetry` dispatches on subcommands (`on`, `off`, `status`, `events`). Adding `report` as a fifth subcommand is the minimal change — no refactor of the dispatch switch. The spec says "add `sworn telemetry [--release <name>] [--json]` report mode that…" — a `report` subcommand matches the established pattern.

### 3. Grouping key: `model_id_confirmed` with fallback to `model`

**Rationale:** S24 added `model_id_confirmed` to capture the response-confirmed model ID (not the configured alias). When `model_id_confirmed` is non-empty (post-S24 dispatches), we use it as the grouping key. When empty (pre-S24 dispatches or dispatches where the response didn't include a model ID), we fall back to `model`. This ensures correct grouping when a configured alias (e.g. `deepseek-v3`) resolves to different actual models and that old dispatches still appear.

### 4. Handling absent/missing fields

- `duration_ms == 0`: exclude from `mean_duration_ms` computation (AC4). This avoids skewing the mean with zeros from pre-S24 dispatches that didn't record duration.
- `input_tokens == 0`: exclude from `mean_input_tokens` computation (same principle).
- `output_tokens == 0`: exclude from `mean_output_tokens` computation.
- `cost_usd`: always summed (0 is a valid cost).

### 5. Rework rate formula

`rework_rate = dispatches_with_attempt_gt_0 / total_dispatches`, expressed as a percentage. `attempt == 0` is the initial dispatch; `attempt > 0` means a re-verify or re-implement triggered by a FAIL verdict.

### 6. Table formatting

Use `fmt.Printf` with fixed-width columns (same style as the existing `telemetry events` table). No dependency on `internal/style` beyond what's already imported — the table is plain text, and `--json` output uses `encoding/json` (stdlib).

## Files touched

- `cmd/sworn/telemetry.go` — add `report` subcommand case, `telemetryReport()` function, and helper functions for the glob walk, aggregation, and formatting.
- `cmd/sworn/telemetry_test.go` — new file (or extend existing); mock status.json files in a temp dir; verify table output and JSON output correctness.

## Design-level risks / reviewer pins

1. **No existing `telemetry_test.go`.** The spec requires a test file. This will be a new file with temp-dir-based fixtures that mock `docs/release/<name>/*/status.json`. The test must not depend on a real release directory on disk. **Pin:** the test setup pattern for mocking `findRepoRoot()` and the filesystem walk is not yet established in this package. I plan to use `t.TempDir()` + `os.MkdirAll` + `state.Write()` to create fixture statuses, then invoke `telemetryReport()` with an explicit root path (injecting the root as a parameter rather than calling `findRepoRoot()` from the test path).

2. **`ModelIDConfirmed` may be empty for post-S24 dispatches.** The S24 spec says `model_id_confirmed` is `omitempty` and populated "from the model's usage response." If a model response doesn't return a model ID in a field the code captures, `model_id_confirmed` will be empty. The fallback to `model` handles this gracefully.

3. **Glob pattern assumes `docs/release/` prefix.** This repo uses `docs/release/` for release artefacts. Some repos use `apps/docs/content/docs/release/` (Fumadocs — see internal/board/oracle.go:132,381,516 for the fallback). The `sworn board` oracle handles this with a fallback check. For the telemetry report, I'll follow the simpler `docs/release/` pattern used by `ledger.go` — both this report and ledger.go only handle `docs/release/`, so the Fumadocs prefix is out of scope for this slice. **Pin:** this is a known gap; the spec only names `docs/release/`.
## AC traceability

| AC | Planned change |
|----|---------------|
| `sworn telemetry --release <name>` compiles and exits 0 with non-empty dispatches | `telemetryReport()` + glob walk |
| Output table: Model, Dispatches, Rework Rate, Mean Input Tokens, Mean Output Tokens, Mean Duration (ms), Total Cost (USD) | `formatTable()` helper |
| `--json` flag → JSON array, one object per model | `--json` flag in `telemetryReport()`; `encoding/json` |
| `duration_ms == 0` excluded from mean | Conditional in aggregation loop |
| `telemetry_test.go`: mock release with two slices, verify rework rate + mean tokens | New test file with temp-dir fixtures |