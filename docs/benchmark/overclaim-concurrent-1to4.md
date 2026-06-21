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
