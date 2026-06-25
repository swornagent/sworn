# Journal — S68-lint-mock

## 2026-07-17 — Implementation

### Decisions

- **Pattern ordering**: Mock and real-infra patterns are ordered by specificity — longer/specific patterns (e.g. `NewMock\w+`, `DATABASE_URL`, `postgres://`) come before general/greedy ones (`mock.`, `localhost:\d+`). First-match-wins per line.
- **Boundary declarations**: Three declaration mechanisms (ACs 3-5): `@mock-boundary` comment in test file, `open_deferrals` entry mentioning mock/stub/boundary/fixture/seed, and `architecture-overrides.json` with `mock-boundary` or `no-mock-boundary` rule IDs.
- **Violations only on intersection**: A mock usage without a real-infra reference is informational only. Violations require both mock AND real-infra patterns in the same file without a boundary declaration.
- **Pattern port**: Ported from `release-mock-check.sh` patterns — Go (`httptest`, `gomock`, `mock.New`), TypeScript (`vi.fn`, `jest.fn`, `sinon`, `nock`, `msw`), Python (`unittest.mock`, `MagicMock`, `patch`, `monkeypatch`).
- **No runtime deps**: Stdlib only — `net/http`, `encoding/json`, `os/exec`, `regexp`.

### Trade-offs

- **First-match per line**: Each line reports only the first matching mock/infra pattern. A line with multiple pattern types only shows the first. Acceptable for lint — the developer can re-run after fixing.
- **File-level granularity**: Boundary declarations (@mock-boundary comment) apply to the whole file, not per-line. A file with a boundary comment anywhere suppresses all violations in that file.

### Out-of-scope discoveries (Rule 2)

None.
## Verifier verdicts received

### 2026-07-17 — Verifier (fresh context)

**Verdict: PASS**

Verification gates:
1. **User-reachable outcome** ✅ — `sworn lint mock` wired at `cmd/sworn/lint.go:55`
2. **Planned touchpoints match actual** ✅ — `mock.go`, `mock_test.go`, `lint.go` (no unexplained files)
3. **Required tests exist** ✅ — `go test ./internal/gate/...` PASS, `go build ./...` PASS, `go vet ./...` PASS
4. **Reachability artefact** ✅ — CLI command produces structured mock usage output
5. **No silent deferrals** ✅ — zero TODO/FIXME patterns in changed code
6. **Scope matches implementation** ✅ — all 6 acceptance checks verified

**Next step:** Track T20-gate-engine has further incomplete slices (S69-lint-regress, S70-llm-check). Next: `/implement-slice S69-lint-regress 2026-06-19-safe-parallelism` in a fresh session.
