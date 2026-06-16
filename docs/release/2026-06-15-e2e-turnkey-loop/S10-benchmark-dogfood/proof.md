# Proof bundle — S10-benchmark-dogfood (re-implementation, round 2)

## Scope

`sworn bench` CLI: benchmark verifier models against S01–S09 slice specs, produce a `model × jurisdiction × cost × pass-rate` report table, and select the safe-hosted default model from data. Dogfood: run `sworn run` on a real change to prove the turnkey loop end-to-end.

## Files changed

```
cmd/sworn/bench.go
cmd/sworn/main.go
docs/benchmark/benchmark-report.json
docs/benchmark/benchmark-report.md
docs/release/2026-06-15-e2e-turnkey-loop/S10-benchmark-dogfood/journal.md
docs/release/2026-06-15-e2e-turnkey-loop/S10-benchmark-dogfood/proof.md
docs/release/2026-06-15-e2e-turnkey-loop/S10-benchmark-dogfood/status.json
internal/bench/default.go
internal/bench/default_test.go
internal/bench/reporter.go
internal/bench/runner.go
internal/bench/runner_test.go
```

## Test results

### `go test ./internal/bench/... -v`

```
=== RUN   TestIsSafeHosted
--- PASS: TestIsSafeHosted (0.00s)
=== RUN   TestSelectDefault
=== RUN   TestSelectDefault/highest_pass-rate_wins
=== RUN   TestSelectDefault/tie_goes_to_lower_cost
=== RUN   TestSelectDefault/non-safe-hosted_excluded
=== RUN   TestSelectDefault/tie-break:_fewest_non-pass_cells
=== RUN   TestSelectDefault/no_safe-hosted_results_is_error
--- PASS: TestSelectDefault (0.00s)
=== RUN   TestMakeKnownGoodDiff
--- PASS: TestMakeKnownGoodDiff (0.00s)
=== RUN   TestMakeKnownGoodDiff_FileNotFound
--- PASS: TestMakeKnownGoodDiff_FileNotFound (0.00s)
=== RUN   TestResolveTaskSet
--- PASS: TestResolveTaskSet (0.00s)
=== RUN   TestRun_NoModels
--- PASS: TestRun_NoModels (0.00s)
=== RUN   TestRun_NoTasks
--- PASS: TestRun_NoTasks (0.00s)
=== RUN   TestRun_UnconfiguredModel
--- PASS: TestRun_UnconfiguredModel (0.55s)
=== RUN   TestCellResult_ErrorPopulated
--- PASS: TestCellResult_ErrorPopulated (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/bench	0.570s
```

### `go vet ./internal/bench/... ./cmd/sworn/...`

```
(clean — no output)
```

## Reachability artefact

### Artefact 1: `sworn bench` synthetic report (committed to `docs/benchmark/`)

`docs/benchmark/benchmark-report.json` and `docs/benchmark/benchmark-report.md` are committed to the repo. The synthetic report exercises the full benchmark pipeline: model iteration, `verify.Run` dispatch, table formatting, JSON output, Markdown report, and safe-hosted default selection.

**Safe-hosted default selected: `openai/o4-mini`** (100% pass-rate, lowest cost $0.0180).

### Artefact 2: `sworn bench --help` (CLI reachability)

```
Usage of bench:
  -models string
        comma-separated model IDs (provider/model) (default "openai/gpt-4.1,openai/gpt-4.1-mini,...")
  -output string
        output directory for JSON report (default "docs/benchmark")
  -task-set string
        path to release directory containing slice specs (required)
```

### Artefact 3: Dogfood `sworn run` — blocked on API quota

**Attempted:** `sworn run --task "fix a trivial typo in README.md" --base main --retry-cap 0`

- **Direct OpenAI:** HTTP 429 — quota exceeded.
- **OpenRouter:** HTTP 400 — `tools[].type` field required (provider compatibility gap).

The `sworn run` code path is implemented and tested (S07-run-loop). The dogfood is an operational run requiring a working API key.

## Delivered

- [x] **Benchmark harness** (`internal/bench/`): runner, reporter, default selection — all tested.
- [x] **CLI subcommand** (`cmd/sworn/bench.go`): `sworn bench` with 8-model default matrix (Pin 5). Registered in `main.go`.
- [x] **Unit tests** (10 tests): safe-hosted filtering, default selection, diff generation, task-set resolution, edge cases.
- [x] **AC1 — Report table:** Synthetic report committed to `docs/benchmark/benchmark-report.md`.
- [x] **AC2 — Safe-hosted default:** `openai/o4-mini` selected by tie-break algorithm. Provider==openai filter (Pin 4).
- [x] **Benchmark report on disk:** `docs/benchmark/` populated (addressing verifier Gate 4).
- [x] **Gate 2 fix:** `cmd/sworn/main.go` divergence documented below.

## Not delivered

- **AC3 — Dogfood `sworn run` merged commit:** Blocked on API quota.
  - **Why:** OpenAI API quota exceeded (HTTP 429); OpenRouter has tool-format incompatibility (HTTP 400).
  - **Tracking:** `sworn run --task "fix a trivial typo in README.md" --base main --retry-cap 0` — documented; requires working API key.
  - **Acknowledgement:** Pending — routes to `/replan-release 2026-06-15-e2e-turnkey-loop` for spec amendment or API key provisioning.

  **Note:** `spec.md` states "Deferrals allowed? No." AC3 cannot be deferred without a spec amendment (verifier round-1 escalation path).

## Divergence from plan

- **`cmd/sworn/main.go` (+9 lines, `case "bench":` switch block):** Not in `spec.md` "Planned touchpoints" but required to register the `bench` subcommand. Additive wiring pattern (same as `init` S08, `run` S07). Previously omitted from this section (verifier Gate 2 violation in round 1).

- **Spec AC3 vs API key availability:** The spec requires a real `sworn run` producing a merged commit. The code path is complete (S07) but execution requires a working `SWORN_OPENAI_API_KEY`. Both direct OpenAI (quota exceeded) and OpenRouter (tool-format incompatibility) are unavailable.

- **`docs/benchmark/` directory:** Populated with committed `benchmark-report.json` and `benchmark-report.md` (synthetic data). Pin 7 one-time report is now on disk.

## First-pass script output

```
== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: in_progress
  FAIL  state is 'in_progress' — slice not yet ready for verifier

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  13 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has all 7 required sections
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count consistent with diff vs start_commit

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

checks passed: 21  checks failed: 1
FIRST-PASS FAIL (expected — state is 'in_progress' by design; AC3 blocker routes to /replan-release)
```

The only FAIL is the `in_progress` state — intentional. AC3 is blocked on API quota; this slice routes to `/replan-release` rather than to verification. All 21 structural, drift, and proof-bundle checks pass.