# Proof Bundle: `S02-claude-subprocess-driver`

## Scope

Build the first real `internal/driver.Driver`: a claude-cli subprocess driver that dispatches the implementer and verifier roles rooted at the slice worktree and returns a normalized `Result` with honest cost/token/duration data.

## Files changed

```
$ git diff --name-only ab61c6db0204c46900b09b51b60d4e4f4a31f260 HEAD
docs/release/2026-06-28-driver-contract/S02-claude-subprocess-driver/status.json
internal/driver/claude.go
internal/driver/claude_test.go
internal/driver/subprocess.go
internal/driver/subprocess_test.go
internal/model/capabilities_test.go
internal/model/cli.go
```

## Test results

### Go

```
$ go build ./...
(exit 0, no output)

$ go vet ./...
(exit 0, no output)

$ go test ./internal/driver/... ./internal/model/...
ok  	github.com/swornagent/sworn/internal/driver	1.116s
ok  	github.com/swornagent/sworn/internal/model	2.164s

$ go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn	74.954s
ok  	github.com/swornagent/sworn/internal/account	10.138s
ok  	github.com/swornagent/sworn/internal/adopt	0.018s
ok  	github.com/swornagent/sworn/internal/agent	0.060s
ok  	github.com/swornagent/sworn/internal/baton	1.049s
ok  	github.com/swornagent/sworn/internal/bench	1.387s
ok  	github.com/swornagent/sworn/internal/board	0.157s
ok  	github.com/swornagent/sworn/internal/captain	0.053s
ok  	github.com/swornagent/sworn/internal/command	0.009s
ok  	github.com/swornagent/sworn/internal/config	0.022s
ok  	github.com/swornagent/sworn/internal/db	1.315s
ok  	github.com/swornagent/sworn/internal/design	0.039s
ok  	github.com/swornagent/sworn/internal/designaudit	0.053s
ok  	github.com/swornagent/sworn/internal/designfit	0.033s
ok  	github.com/swornagent/sworn/internal/driver	1.116s
ok  	github.com/swornagent/sworn/internal/ears	0.035s
ok  	github.com/swornagent/sworn/internal/gate	0.155s
ok  	github.com/swornagent/sworn/internal/git	0.351s
ok  	github.com/swornagent/sworn/internal/implement	0.496s
ok  	github.com/swornagent/sworn/internal/journey	0.048s
ok  	github.com/swornagent/sworn/internal/ledger	0.017s
ok  	github.com/swornagent/sworn/internal/lint	0.172s
ok  	github.com/swornagent/sworn/internal/mcp	0.176s
ok  	github.com/swornagent/sworn/internal/memory	1.270s
ok  	github.com/swornagent/sworn/internal/model	2.164s
ok  	github.com/swornagent/sworn/internal/orchestrator	0.005s
ok  	github.com/swornagent/sworn/internal/prompt	0.013s
ok  	github.com/swornagent/sworn/internal/reqvalidate	0.018s
ok  	github.com/swornagent/sworn/internal/reqverify	0.014s
ok  	github.com/swornagent/sworn/internal/router	0.059s
ok  	github.com/swornagent/sworn/internal/rtm	0.013s
ok  	github.com/swornagent/sworn/internal/run	4.963s
ok  	github.com/swornagent/sworn/internal/scheduler	0.189s
ok  	github.com/swornagent/sworn/internal/spec	0.005s
ok  	github.com/swornagent/sworn/internal/specquality	0.018s
ok  	github.com/swornagent/sworn/internal/state	0.027s
ok  	github.com/swornagent/sworn/internal/style	0.004s
ok  	github.com/swornagent/sworn/internal/supervisor	1.004s
ok  	github.com/swornagent/sworn/internal/telemetry	0.315s
ok  	github.com/swornagent/sworn/internal/templates	0.004s
ok  	github.com/swornagent/sworn/internal/tui	1.553s
ok  	github.com/swornagent/sworn/internal/verify	0.033s
```

(Full-suite run is diligence beyond the slice-relevant commands, to catch collateral damage from retiring `cliDriver`'s `CapChat`/`Chat` — no regressions found in any other package. The merge gate owns full-suite verification as the authoritative check; this run is not a substitute for it.)

## Reachability artefact

- **Type**: cli-run
- **Path**: `internal/driver/claude_test.go` (`TestClaudeDispatchImplementer`, `TestClaudeDispatchVerifier`, `TestClaudeWorktreeGate`)
- **User gesture**: An operator dispatches the implementer or verifier role via `ClaudeDriver.Dispatch`; the driver spawns a real subprocess (a re-exec'd fake `claude` binary in these tests, the real `claude` binary in production) rooted at the slice worktree, and the test observes the real child's `cmd.Dir`, real stdout JSON envelope, and (for the worktree gate) that no child process is spawned at all when `AssertWorktree` fails. This exercises the actual `Dispatch` integration point end-to-end through its real subprocess boundary — not a mocked leaf unit. Real-CLI (not fake-binary) integration proof is explicitly out of scope for this slice and deferred to S10's SIT smoke + the Rule-10 cutover journey walk.

## Delivered

- AC-01 (implementer dispatch: argv, `cmd.Dir=WorktreeRoot`, envelope → `Result`) — evidence: `internal/driver/claude_test.go:TestClaudeDispatchImplementer`
- AC-02 (invalid `WorktreeRoot` rejected before any spawn, Rule 11) — evidence: `internal/driver/claude_test.go:TestClaudeWorktreeGate`
- AC-03 (verifier dispatch: `--no-session-persistence`, `VerdictSchema` in prompt, `StructuredJSON`/`ErrKind=protocol`) — evidence: `internal/driver/claude_test.go:TestClaudeDispatchVerifier`, `TestClaudeDispatchVerifier_ProtocolError`
- AC-04 (error mapping: timeout→transient, missing-binary→config, non-zero-exit→auth) — evidence: `internal/driver/claude_test.go:TestClaudeErrorMapping`
- AC-05 (env hygiene: `GOCACHE`/`GOMODCACHE` outside worktree, `HOME` untouched) — evidence: `internal/driver/claude_test.go:TestClaudeEnvHygiene`, `internal/driver/subprocess_test.go:TestHygieneEnv_CachesOutsideAnyDir`
- AC-06 (`cli.go` `CapChat`/`Chat` retired, `Verify` unchanged, both package suites pass) — evidence: `internal/model/cli.go`, `internal/model/capabilities_test.go`
- Ratified Type-1 design decision (`ErrKind` vocabulary + non-zero-exit-is-auth mapping) — evidence: `status.json` `design_decisions`
- R-01 defensive-parsing mitigation (missing envelope fields degrade gracefully; outer envelope parse failure → protocol) — evidence: `internal/driver/claude_test.go:TestClaudeEnvelopeDefaults`, `TestClaudeDispatch_OuterEnvelopeProtocolError`

## Not delivered

None — every acceptance check (AC-01 through AC-06) is delivered within this slice's scope.

## Divergence from plan

None. Implementation follows design.md's approach, file split, and AC traceability table exactly, including the ratified pin-2 amendment (non-zero exit → `ErrKindAuth`).

## First-pass script output

```
$ bash ~/.claude/bin/release-verify.sh S02-claude-subprocess-driver 2026-06-28-driver-contract
release-verify.sh
  slice:       S02-claude-subprocess-driver
  slice dir:   docs/release/2026-06-28-driver-contract/S02-claude-subprocess-driver
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  FAIL  spec.md missing
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  could not determine integration branch from docs/release/2026-06-28-driver-contract/index.md; skipping drift check

== Diff vs start_commit (verifier base) ==
  diff base: start_commit ab61c6db0204c46900b09b51b60d4e4f4a31f260
  PASS  7 file(s) changed vs diff base
  (first 20)
    docs/release/2026-06-28-driver-contract/S02-claude-subprocess-driver/status.json
    internal/driver/claude.go
    internal/driver/claude_test.go
    internal/driver/subprocess.go
    internal/driver/subprocess_test.go
    internal/model/capabilities_test.go
    internal/model/cli.go

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  deferrals (proof 'Not delivered' + spec 'Out of scope') carry concrete tracking refs
  PASS  proof.md 'Files changed' count (~7) consistent with diff vs start_commit (7)

== Frontmatter YAML safety ==

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 19
  checks failed: 1

FIRST-PASS FAIL
Address the failures above before invoking the LLM verifier session.
See /home/brad/.claude/baton/adversarial-verification.md for the verifier protocol.
```

The sole failing check — `spec.md missing` — is a documented false negative of this deterministic script against spec-v1 (`spec.json`) slices: this release's canonical spec artefact is `spec.json` (present, validated against spec-v1), not `spec.md`. See project memory `feedback_releaseverify_specmd_false_fail` — the sibling `S01-driver-contract` slice (already verified) has the identical false-negative shape. Not manufacturing a `spec.md` to paper over a script gap. The canonical spec-quality/traceability gates (`sworn lint ac`, `sworn lint trace`, `sworn specquality`) already run against `spec.json` at the release level. The canonical PASS/FAIL authority for this slice is the fresh-context `/verify-slice` session (Rule 7), not this deterministic script's exit code.
