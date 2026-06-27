# Design TL;DR — S10-benchmark-dogfood

## §1. User-visible change

`sworn bench` runs a benchmark: iterate candidate models against this release's own
verified slice specs (S01–S09), record pass-rate + cost + jurisdiction, and pick
the safe-hosted default model from data — no non-trusted-hosted model blessed.
Then a real `sworn run` dogfood lands a verified, merged change on this repo,
proving the turnkey loop end-to-end. The benchmark report lives at `docs/benchmark/`.

## §2. Design decisions not in spec (max 5)

1. **Task set — self-dogfood.** Use this release's own S01–S09 slice specs as the
   public task set. They are already verified, cover the full surface area (verify
   core → distribution), and are publicly visible. Self-dogfood is the strongest
   proof.
2. **Model list — OpenAI baseline.** Benchmark matrix: `openai/gpt-4.1`,
   `openai/gpt-4.1-mini`, `openai/gpt-4.1-nano`, `openai/gpt-4o`,
   `openai/gpt-4o-mini`, `openai/o4-mini`, `openai/o3`, `openai/o3-mini`.
   All `openai` = safe-hosted (trusted-jurisdiction US). Additional providers gated
   behind explicit `SWORN_<PROVIDER>_API_KEY` + `BASE_URL`.
3. **Default selection algorithm.** Highest pass-rate among safe-hosted models. Tie
   → lowest average cost. Tie → fewest API calls. Encoded in
   `internal/bench/default.go`.
4. **Benchmark architecture.** `internal/bench/` calls the existing `verify.Run`
   func per model×task — deterministic, no reimplementation of the loop. A
   separate reporter writes a table to stdout and a JSON report to
   `docs/benchmark/`.
5. **Dogfood run.** `sworn run --task "<trivial change>" --base main` — a real
   turnkey loop on this repo. The merged commit + run transcript is the
   reachability artefact. One attempt (no retry ladder needed — the system is
   already proven on S01–S09).

## §3. Files I'll touch grouped by purpose

- `internal/bench/` — benchmark engine: `runner.go` (iterate models×tasks),
  `reporter.go` (tabulate), `default.go` (select safe-hosted default).
- `cmd/sworn/bench.go` — CLI subcommand: `sworn bench [--task-set <dir>]
  [--models <comma-sep>] [--output <dir>]`.
- `cmd/sworn/main.go` — register `bench` subcommand (additive dispatch entry;
  documented shared file per touchpoint matrix).
- `docs/benchmark/` — benchmark report output (JSON + markdown).

## §4. Things I'm NOT doing

- No publishing or marketing of benchmark results.
- No CI integration for continuous benchmarking.
- No cost-cap enforcement in the benchmark (handled by per-run `--retry-cap` in
  the dogfood; benchmark runs are single-attempt, no retry).
- No multi-release benchmark comparison (v0.1 only).

## §5. Reachability plan

1. `sworn bench --task-set docs/release/2026-06-15-e2e-turnkey-loop/
   --models openai/gpt-4.1,openai/gpt-4o-mini` → tabular output to stdout +
   JSON report at `docs/benchmark/`. Screenshot/capture of the table output.
2. `sworn run --task "fix a trivial typo in README.md" --base main`
   → lands a verified, merged commit. The merged commit SHA + run transcript
   is the reachability artefact.

## §6. Open questions for the Coach

- Which exact models should be in the initial benchmark matrix? The 8 OpenAI
  models proposed above, or a subset?
- How many retries per model×task in the benchmark? Proposed: 1 (single attempt
  — the benchmark measures first-pass success rate, which is the relevant metric
  for the turnkey experience).
- Should the benchmark report be committed as a one-time artefact or kept as a
  CI-regenerated file? Proposed: one-time committed report (this is a v0.1
  launch proof, not a dashboard).
- Is a trivial change (typo/README) acceptable as the dogfood proof-of-loop, or
  does it need to exercise the tool on a nontrivial code change?