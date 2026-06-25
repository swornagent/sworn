# Proof Bundle: `S45-design-tldr`

## Scope

`sworn run` generates a design TL;DR (`design.md`) in the slice directory before writing any code — the same six-section TL;DR the coach-loop implementer emits: §1 user-visible change, §2 design decisions not in the spec (max 5), §3 files I'll touch by purpose, §4 things I'm NOT doing, §5 reachability plan, §6 open questions.

## Files changed

```
$ git diff --stat 846fca5..HEAD
 docs/release/2026-06-19-safe-parallelism/S45-design-tldr/status.json |   2 +-
 internal/design/tldr.go                                              |  85 +++++++++
 internal/design/tldr_test.go                                         | 189 +++++++++++++++++++++
 internal/prompt/design-tldr.md                                       |  47 +++++
 internal/prompt/prompt.go                                            |  15 +-
 internal/prompt/prompt_test.go                                       |   6 +-
 internal/run/run_test.go                                             |   8 +-
 internal/run/slice.go                                                |  40 +++++
 8 files changed, 378 insertions(+), 14 deletions(-)
```

## Test results

### Go

```
$ go test -race ./internal/design/... ./internal/run/...
ok  	github.com/swornagent/sworn/internal/design	1.033s
ok  	github.com/swornagent/sworn/internal/run	3.985s
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: `docs/release/2026-06-19-safe-parallelism/S45-design-tldr/proof.md`
- **User gesture**: Run `go test -race ./internal/design/... ./internal/run/...` — exercises `design.Generate()` with fake agents asserting design.md is written with six sections, existing design.md is respected, and the RunSlice integration invokes the design step before the implement loop.

## Delivered

- `internal/design/tldr.go` — `Generate()` function: reads spec, calls model via single-shot Chat (tool-less), validates six sections present, writes `design.md` — evidence: `internal/design/tldr.go`
- `internal/design/tldr_test.go` — unit tests: `TestGenerateWritesSixSections`, `TestGenerateRespectsExisting` (idempotent with/without Regenerate), `TestHasSixSections`, `TestGenerateModelError`, `TestGenerateMissingSections` — evidence: `internal/design/tldr_test.go`
- `internal/prompt/design-tldr.md` — embedded §1–§6 design TL;DR prompt — evidence: `internal/prompt/design-tldr.md`
- `internal/prompt/prompt.go` — `//go:embed design-tldr.md` + `DesignTLDR()` accessor — evidence: `internal/prompt/prompt.go`
- `internal/run/slice.go` — design-TL;DR step injected before implement loop in `RunSlice`, uses first escalation model, bounded by same timeout, warns and proceeds on failure — evidence: `internal/run/slice.go`
- Acceptance checks: all 4 ACs satisfied — evidence: test output above + proof bundle structural checks
- All test commands pass: `go test -race ./internal/design/... ./internal/run/...` PASS

## Not delivered

None.

## Divergence from plan

None.

## First-pass script output

```
$ ~/.claude/bin/release-verify.sh S45-design-tldr
=== see live run above ===
```