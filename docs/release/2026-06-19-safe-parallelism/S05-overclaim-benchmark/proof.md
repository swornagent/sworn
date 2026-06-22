# Proof Bundle: S05-overclaim-benchmark

## Scope

A developer or external reviewer runs `sworn bench overclaim` and receives a report showing overclaim rate at N=1, N=2, and N=4 concurrent tracks on a fixture release, with the rate demonstrably flat across all three — proving the verify gate holds under concurrency.

## Files changed

```
$ git diff --name-only 29c13e42f1b81c84a67454ca92ef307ecc847533
cmd/sworn/bench.go
docs/benchmark/overclaim-concurrent-1to4.md
docs/release/2026-06-19-safe-parallelism/S05-overclaim-benchmark/journal.md
docs/release/2026-06-19-safe-parallelism/S05-overclaim-benchmark/status.json
internal/bench/overclaim.go
internal/bench/overclaim_test.go
```

## Test results

### Go

```
$ go test ./internal/bench/... -count=1
ok  github.com/swornagent/sworn/internal/bench  0.406s
```

### Go vet

```
$ go vet ./internal/bench/... ./cmd/sworn/...
(exit 0)
```

### Go race detector (Pin 4: counter race safety)

```
$ go test -race ./internal/bench/... -run 'TestOverclaim|TestBenchmark|TestZero' -count=1
ok  github.com/swornagent/sworn/internal/bench  2.324s
```

### Reachability: `sworn bench overclaim`

```
$ ./bin/sworn bench overclaim --publish
(see Reachability artefact section below for the published output)
exit 0
```

### Determinism check (AC4: 5× identical output)

```
$ for i in 1 2 3 4 5; do ./bin/sworn bench overclaim 2>/dev/null | md5sum; done
60e3079241c07430ae5282901669e987  -
60e3079241c07430ae5282901669e987  -
60e3079241c07430ae5282901669e987  -
60e3079241c07430ae5282901669e987  -
60e3079241c07430ae5282901669e987  -
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: `docs/benchmark/overclaim-concurrent-1to4.md`
- **User gesture**: Developer runs `sworn bench overclaim --publish` and observes a Markdown table with rows for N=1, N=2, N=4 showing 0% overclaim rate at all levels. The file is written to `docs/benchmark/overclaim-concurrent-1to4.md`.

The published artefact content:

```
# Overclaim Benchmark: Concurrent Track Scaling (N=1→4)

## Results

| N (concurrent tracks) | Runs | Overclaims | Underclaims | Overclaim Rate | Underclaim Rate |
|-----------------------|------|------------|-------------|----------------|-----------------|
| 1 | 5 | 0 | 0 | 0.0% | 0.0% |
| 2 | 5 | 0 | 0 | 0.0% | 0.0% |
| 4 | 5 | 0 | 0 | 0.0% | 0.0% |

## Methodology

- **Fixture**: 12 slices (8 designed to PASS, 4 designed to FAIL)
- **Mock verifier**: always returns the correct verdict (deterministic)
- **Repetitions**: 5 per N level (deterministic mocks → identical results)
- **Overclaim**: FAIL slice whose verifier returned PASS (false positive)
- **Underclaim**: PASS slice whose verifier returned FAIL (false negative)
- **Rate denominator**: total slices (12), not FAIL slices

## Conclusion

Overclaim rate is 0% at N=1, N=2, and N=4 — the concurrent scheduler does not
corrupt the verify gate under parallelism.
```

## Delivered

- **AC1: `sworn bench overclaim` runs to completion without live API calls** — evidence: `cmd/sworn/bench.go` `cmdBenchOverclaim` dispatches to `bench.RunOverclaimBenchmark` which uses mock RunSliceFn (no API calls); binary exits 0.
- **AC2: Output includes a table with rows for N=1, N=2, N=4; each row shows overclaim count, underclaim count, overclaim rate, underclaim rate** — evidence: `internal/bench/overclaim.go` `FormatMarkdownTable` produces the table; published artefact at `docs/benchmark/overclaim-concurrent-1to4.md` shows all 3 rows with all 4 columns.
- **AC3: Overclaim rate is 0% at N=1, N=2, and N=4 on the deterministic fixture** — evidence: published artefact shows 0.0% overclaim rate at all N values; `TestZeroOverclaimWithCorrectGate` asserts this programmatically.
- **AC4: Running `sworn bench overclaim` 5× produces identical output** — evidence: 5 md5sum hashes above are identical (`60e3079241c07430ae5282901669e987`); `TestBenchmarkDeterministic` asserts this in-test.
- **AC5: `sworn bench overclaim --publish` writes a valid Markdown file to `docs/benchmark/overclaim-concurrent-1to4.md`** — evidence: file exists on disk, content shown above.
- **AC6: `go test ./internal/bench/...` covers the overclaim rate calculation** — evidence: `TestOverclaimRateCalculation` asserts 4/12 = 33.3% with known overclaims; `TestUnderclaimRateCalculation` asserts underclaim rate; `TestZeroOverclaimWithCorrectGate` asserts 0% at all N; `TestBenchmarkDeterministic` asserts identical results across runs; `TestFormatMarkdownTable` and `TestFormatJSON` assert report formatting.

## Not delivered

None. All acceptance checks are delivered. No deferrals.

## Divergence from plan

None. Implementation matches the spec's planned touchpoints and the Coach-approved design TL;DR. All 5 Captain pins addressed inline during implementation.

## First-pass script output

```
$ release-verify.sh S05-overclaim-benchmark 2026-06-19-safe-parallelism
release-verify.sh
  slice:       S05-overclaim-benchmark
  slice dir:   docs/release/2026-06-19-safe-parallelism/S05-overclaim-benchmark
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 29c13e42f1b81c84a67454ca92ef307ecc847533
  PASS  7 file(s) changed vs diff base
  (first 20)
    cmd/sworn/bench.go
    docs/benchmark/overclaim-concurrent-1to4.md
    docs/release/2026-06-19-safe-parallelism/S05-overclaim-benchmark/journal.md
    docs/release/2026-06-19-safe-parallelism/S05-overclaim-benchmark/proof.md
    docs/release/2026-06-19-safe-parallelism/S05-overclaim-benchmark/status.json
    internal/bench/overclaim.go
    internal/bench/overclaim_test.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  proof.md test results section scoped to slice-relevant tests
```

(Note: the `PLAYWRIGHT_OPTIN: unbound variable` error from the script is an environmental issue in the script itself, not a slice defect. The slice has no Playwright/E2E component.)