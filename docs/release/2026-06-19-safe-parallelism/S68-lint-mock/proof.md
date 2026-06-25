# Proof Bundle: `S68-lint-mock`

## Scope

A developer runs `sworn lint mock --slice <id> --release <name>` and receives a report of any undeclared mock boundaries: test files using mocks/stubs/fixtures/seeded data without declaring the boundary. Declared boundaries (via `@mock-boundary` comment, `open_deferrals`, or `architecture-overrides.json`) are accepted. Exits 0 on clean, 1 with violations.

## Files changed

```
$ git diff --name-only f11f9d8908d6652014afb079036260074753ac50..HEAD
cmd/sworn/lint.go
docs/release/2026-06-19-safe-parallelism/S68-lint-mock/journal.md
docs/release/2026-06-19-safe-parallelism/S68-lint-mock/status.json
internal/gate/mock.go
internal/gate/mock_test.go
```

## Test results

### Go

```
$ go test -count=1 ./internal/gate/...
ok  	github.com/swornagent/sworn/internal/gate	0.038s
```

### Build

```
$ go build ./...
(exit 0, no output)
```

### Lint (vet)

```
$ go vet ./...
(exit 0, no output)
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: N/A — CLI command output
- **User gesture**: Run `sworn lint mock --slice S68-lint-mock --release 2026-06-19-safe-parallelism` from the worktree root. Observe structured output listing mock usages and violations.

```
$ go run ./cmd/sworn lint mock --slice S68-lint-mock --release 2026-06-19-safe-parallelism

MOCK LINT — 2026-06-19-safe-parallelism / S68-lint-mock

Mock/stub/fixture usages found: 0

No undeclared mock boundaries.

PASS — mock lint clean
```

```
$ go run ./cmd/sworn lint mock --slice S66-lint-coverage --release 2026-06-19-safe-parallelism --json
{
  "slice": "S66-lint-coverage",
  "release": "2026-06-19-safe-parallelism",
  "mock_usages": [
    {"file":"internal/gate/mock_test.go","line":19,"kind":"httptest","value":"httptest."},
    {"file":"internal/gate/mock_test.go","line":22,"kind":"NewMock","value":"NewMockClient"},
    {"file":"internal/gate/mock_test.go","line":25,"kind":"stub.New","value":"stub.New"},
    {"file":"internal/gate/mock_test.go","line":26,"kind":"mock.New","value":"mock.New"},
    {"file":"internal/gate/mock_test.go","line":29,"kind":"vitest-mock","value":"vi.fn"},
    {"file":"internal/gate/mock_test.go","line":44,"kind":"testdata/","value":"testdata/"},
    {"file":"internal/gate/archrules_test.go","line":403,"kind":"inline-json-stub","value":"json.Unmarshal([]byte"},
    {"file":"internal/gate/design_test.go","line":255,"kind":"inline-json-stub","value":"json.Unmarshal([]byte"}
  ],
  "violations": null,
  "total_violations": 0,
  "verdict": "PASS"
}
```

## Delivered

- **Detects mock/stub/fixture/seed usage in test files** — evidence: `internal/gate/mock.go` `findMocksInFile` with 30+ mock patterns covering Go (httptest, gomock, mock.New, testify/mock), TypeScript (vi.fn, jest.fn, sinon, nock, msw), Python (unittest.mock, MagicMock, patch, monkeypatch, responses), plus testdata/fixtures directories
- **Detects real-infra references alongside mocks** — evidence: `internal/gate/mock.go` `findInfraInFile` with 40+ infra patterns: localhost, DATABASE_URL, POSTGRES, MYSQL, AUTH0_DOMAIN, STRIPE_KEY, process.env, os.Getenv, http.Get, fetch, axios, aws-sdk, etc.
- **Accepts `@mock-boundary` comments as declared boundaries** — evidence: `internal/gate/mock.go` `hasMockBoundaryInFile` + `mock_test.go` `TestHasMockBoundaryInFile`
- **Accepts `open_deferrals` entries mentioning mocks** — evidence: `internal/gate/mock.go` `MockDeferrals.HasMockBoundary` + `mock_test.go` `TestMockDeferralsHasMockBoundary` (checks what/why fields for mock/stub/boundary/fixture/seed)
- **Accepts `architecture-overrides.json` suppressed rules** — evidence: `internal/gate/mock.go` `isMockExempt` (checks mock-boundary and no-mock-boundary rule IDs) + `mock_test.go` `TestIsMockExempt`
- **Exits 0 on clean, 1 with violations** — evidence: `cmd/sworn/lint.go` `cmdLintMock` returns 0/1 via `report.HasViolations()`, `mock_test.go` `TestMockReportJSON` / `TestPrintMockPass` / `TestPrintMockFail`

## Not delivered

None — all six acceptance checks are delivered.

## Divergence from plan

None — implementation matches spec exactly: `internal/gate/mock.go`, `internal/gate/mock_test.go`, `cmd/sworn/lint.go` all created/extended as planned.

## First-pass script output

```
$ /home/brad/.claude/bin/release-verify.sh S68-lint-mock 2026-06-19-safe-parallelism
FIRST-PASS PASS (23/23 checks) — see full output below.
```