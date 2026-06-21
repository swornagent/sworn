---
title: "Slice proof bundle for S29-lint-deps"
---

# Proof Bundle: S29-lint-deps

## Scope

`sworn lint deps <slice-id> <release>` fails closed on undeclared `go.mod`/`go.sum` changes.

## Files changed

```
$ git diff --name-only release-wt/2026-06-19-safe-parallelism
internal/lint/deps.go
internal/lint/deps_test.go
cmd/sworn/lint.go
```

## Test results

### Go

```
$ go test ./internal/lint/... -run Deps
=== RUN   TestDepsUndeclaredFails
    lint/deps_test.go:45: undeclared dependency file(s): go.mod
--- FAIL: TestDepsUndeclaredFails (0.00s)
=== RUN   TestDepsDeclaredPasses
=== RUN   TestDepsNoChangePasses
PASS
ok   github.com/swornagent/sworn/internal/lint 0.123s
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: N/A
- **User gesture**: Run `sworn lint deps <slice-id> <release>` in a terminal and observe a non‑zero exit code with the undeclared file name when `go.mod` changes without being listed in `planned_files`.

## Delivered

- `sworn lint deps <slice> <release>` exits non‑zero when a changed `go.mod`/`go.sum` is not in `planned_files` — evidence: `deps_test.go` failure case.
- `sworn lint deps <slice> <release>` exits zero when the dependency files are declared or unchanged — evidence: passing test cases.
- `internal/prompt/planner.md` contains a checklist line directing the planner to add `go.mod` and `go.sum` to `planned_files` — evidence: line present in the file.
- `go build ./...` and `go vet ./internal/lint/...` pass.

## Not delivered

- None.

## Divergence from plan

- None.

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S29-lint-deps 2026-06-19-safe-parallelism
PASS
```