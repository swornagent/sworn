---
title: 'S05-overclaim-benchmark — overclaim rate flat 1→4 concurrent tracks'
description: 'Formal repeatable benchmark proving overclaim rate is flat as concurrent track count scales from 1 to 4. Published as a committed release artefact.'
---

# Slice: `S05-overclaim-benchmark`

## User outcome

A developer or external reviewer runs `sworn bench overclaim` and receives a report
showing overclaim rate at N=1, N=2, and N=4 concurrent tracks on a fixture release,
with the rate demonstrably flat across all three — proving the verify gate holds under
concurrency.

## Entry point

`sworn bench overclaim` subcommand; `sworn bench overclaim --publish` writes results
to `docs/benchmark/overclaim-concurrent-1to4.md`.

## In scope

- **Fixture release**: a synthetic release with 12 slices — 8 designed to PASS
  verification and 4 designed to FAIL — using pre-baked mock verifier responses (no
  live API calls; fully deterministic)
- **Overclaim definition**: a FAIL slice whose verifier returned PASS (false positive).
  The fixture knows the ground truth for each slice.
- **Underclaim definition**: a PASS slice whose verifier returned FAIL (false negative).
  Measured but less dangerous; reported separately.
- **Benchmark harness** (`internal/bench/overclaim.go`):
  - Runs the fixture release through the verify gate at N=1, N=2, and N=4 concurrency
    (using the concurrent scheduler from S02 with mock implementer + verifier)
  - Counts overclaims and underclaims per run
  - Repeats 5× at each N and averages (deterministic mocks → same result each time;
    repetitions confirm no non-determinism was introduced)
  - Produces a result struct: `{N, runs, overclaim_count, underclaim_count,
    overclaim_rate, underclaim_rate}`
- **Report format**: JSON (for machine consumption) + Markdown table (for humans)
- `sworn bench overclaim --publish` writes the Markdown table to
  `docs/benchmark/overclaim-concurrent-1to4.md` and commits it

## Out of scope

- Live API calls in the benchmark (all mocked; the benchmark tests scheduler+gate
  correctness, not model quality — the model benchmark is existing `sworn bench` scope)
- N>4 concurrency in R3 (the SQLite schema supports it; the benchmark can be extended)
- Latency or throughput measurements

## Planned touchpoints

- `internal/bench/overclaim.go` (new — fixture generator + harness + calculator)
- `internal/bench/overclaim_test.go` (new)
- `cmd/sworn/bench.go` (touch — add `overclaim` subcommand)
- `docs/benchmark/overclaim-concurrent-1to4.md` (new — published artefact, committed
  by `--publish` or manually after first run)

## Acceptance checks

- [ ] `sworn bench overclaim` runs to completion without live API calls (all mock)
- [ ] Output includes a table with rows for N=1, N=2, N=4; each row shows overclaim
  count, underclaim count, overclaim rate, underclaim rate
- [ ] Overclaim rate is 0% at N=1, N=2, and N=4 on the deterministic fixture (the mock
  verifier never returns wrong verdicts; the test proves the gate itself doesn't corrupt)
- [ ] Running `sworn bench overclaim` 5× produces identical output (determinism check)
- [ ] `sworn bench overclaim --publish` writes a valid Markdown file to
  `docs/benchmark/overclaim-concurrent-1to4.md`
- [ ] `go test ./internal/bench/...` covers the overclaim rate calculation: given known
  pass/fail ground truth and known verifier responses, the rate calculation is correct

## Required tests

- **Unit**: `internal/bench/overclaim_test.go`
  — `TestOverclaimRateCalculation`: fixture with 4 known overclaims out of 12 slices;
    assert calculated rate is 4/12 = 33.3%; assert underclaim calculation is correct
  — `TestBenchmarkDeterministic`: run the benchmark harness twice; assert identical
    results (proves no randomness or race)
  — `TestZeroOverclaimWithCorrectGate`: fixture where verifier always returns correct
    verdict; assert overclaim_rate == 0.0 at all N values
- **Reachability artefact**: `sworn bench overclaim --publish` executed; the committed
  `docs/benchmark/overclaim-concurrent-1to4.md` file is the artefact. Include the
  file path in proof.md.

## Risks

- The benchmark's value depends on the fixture being representative. A trivially-easy
  fixture could pass while a real parallel run fails. Mitigation: the fixture includes
  at least 4 FAIL slices and at least 2 PASS slices that could be confused by a
  stateful verifier (similar spec content, same worktree root). This exercises the
  isolation invariant S03 proves.

## Deferrals allowed?

No. `docs/benchmark/overclaim-concurrent-1to4.md` is a launch-gate requirement per
the ROADMAP. Without it, the parallelism claim is unproven.
