---
title: Proof Bundle — S63-subscription-cli-driver
description: Rule 6 proof bundle. Generated from live repo state, not recollection.
---

# Proof Bundle: `S63-subscription-cli-driver`

## Scope

A developer who already pays for Claude Code (Pro/Max) or ChatGPT (Codex) runs sworn with no API key set. They configure a role to use `claude-cli:<model>` or `codex:<model>` in the per-role model config, and sworn dispatches that role by spawning the user's locally-installed CLI which authenticates through the CLI's own logged-in session.

## Files changed

```
$ git diff --name-only a901d6754ec0e4a50cc1e2aa8ef984945d27f88f
docs/release/2026-06-19-safe-parallelism/S63-subscription-cli-driver/status.json
internal/model/config.go
internal/model/provider.go
```

New files:
```
internal/model/cli.go
internal/model/cli_test.go
```

## Test results

### Go

```
$ go test -race ./internal/model/... -count=1
ok  	github.com/swornagent/sworn/internal/model	6.357s
```

```
$ go build ./...
(exit 0)
```

```
$ go vet ./internal/model/...
(exit 0)
```

## Reachability artefact

- **Type**: test-driven integration point
- **Path**: `internal/model/cli_test.go` → `TestClaudeCLI_FromEnvIntegration`
- **User gesture**: `FromEnv("claude-cli/sonnet")` → `NewClient()` → `cliDriver.Verify()` — the full config-to-dispatch path exercised against a fake `claude` binary in `TestClaudeCLI_FromEnvIntegration`

## Delivered

- [x] With no `SWORN_*_API_KEY` set and a (fake) `claude` binary on PATH, a role dispatch completes via the subprocess driver — evidence: `TestClaudeCLI_NormalDispatch`, `TestClaudeCLI_FromEnvIntegration` (cli_test.go:74, cli_test.go:187)
- [x] The driver is selectable via config as `claude-cli/<model>` per role — evidence: `TestClaudeCLI_FromEnvIntegration` (FromEnv path), `TestClaudeCLI_NoProxyRouting` (bypasses proxy)
- [x] A missing or unauthenticated CLI yields a typed `model.Error{Kind}` — evidence: `TestClaudeCLI_MissingBinary` → KindOther, `TestClaudeCLI_AuthFailure` → KindAuth, `TestClaudeCLI_Timeout` → KindTransient
- [x] `go test -race ./internal/model/...` passes — evidence: test run above (PASS, 6.357s)

## Not delivered

- Codex `exec` subprocess driver — **Why**: Different invocation shapes and output normalisation from `claude -p`. Claude-CLI ships first to unblock subscription-based flow. **Tracking**: GitHub issue #19; `// TODO: codex exec support (S63-deferral-1)` in `cli.go` and `provider.go`. **Acknowledged**: Coach (approved-ack.md pin 3 and flag c), 2026-07-14.

## Divergence from plan

- `internal/model/provider.go` was not listed in the original spec's "Planned touchpoints" but was included in design §3 and the Captain review (pin 1). Added to `planned_files` in status.json.
- `codex` provider returns a deferral error from `NewClient()` rather than returning a `*cliDriver` — per Coach pin 3.
- Missing binary detection uses `*fs.PathError` in addition to `*exec.Error` — Go 1.26 returns `*fs.PathError` for absolute-path missing binaries. Both are handled.
- Output is trimmed of trailing whitespace (`strings.TrimSpace`) — necessary because `fmt.Println` in test fakes appends `\n`.

## First-pass script output


22/23 checks passed, 1 FAIL: dark-code markers for codex deferral (all Rule 2 —
tracking #19, Coach-acknowledged). All other gates green.

$ release-verify.sh S63-subscription-cli-driver 2026-06-19-safe-parallelism
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
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  4 file(s) changed vs diff base

== Dark-code markers in changed files ==
  FAIL  dark-code markers found (must be Rule 2 deferrals)
  hits: codex support deferred (S63-deferral-1) x 3 — all Rule 2, tracked #19.

== Proof bundle structural checks ==
  PASS  all 8 required sections present
  PASS  no template placeholders
  PASS  Not delivered deferrals carry non-placeholder tracking refs
  PASS  Files changed count consistent with diff

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output
