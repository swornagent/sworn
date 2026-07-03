# Design TL;DR — S02-claude-subprocess-driver

## User outcome (from spec.json)

An operator on a Claude subscription can dispatch the implementer and
verifier roles through a claude-cli subprocess driver that delegates the
entire agentic loop to the CLI, runs the child process inside the slice
worktree, and returns a normalized `Result` with honest cost/token/duration
data — the sworn#35 class (tools silently ignored, child running in the wrong
directory) is gone by construction.

## Approach

Build the first real implementation of `driver.Driver` (from S01,
`internal/driver/driver.go`, merged and verified). Split the work into two
files so the plumbing S03 (codex) needs is reusable without copy-paste:

- **`internal/driver/subprocess.go`** — provider-neutral subprocess plumbing:
  spawn-with-timeout, env hygiene, and error classification into this
  package's own `ErrKind` string vocabulary. Nothing claude-specific lives
  here.
- **`internal/driver/claude.go`** — `ClaudeDriver`, the `driver.Driver`
  implementation: builds the `claude -p` argv, calls the shared spawn
  helper, parses the CLI's `--output-format json` envelope into `Result`.

Each dispatch is exactly one subprocess call — for both roles. The
implementer role's entire agentic loop (multi-turn tool use) happens *inside*
the `claude` CLI process; sworn does not orchestrate turns for this driver
(that's the point of "delegates the entire agentic loop to the CLI" — it is
categorically different from the in-process driver S04 builds, which does
own the turn loop via `internal/agent`).

`internal/model/cli.go` (`cliDriver`) is *not* deleted this slice — its
one-shot `Verify` stays as the decided utility path (2026-07-02 pin). Only
its dishonest `CapChat`/`Chat` surface is retired (AC-06).

## Key design choices + rationale

1. **New `ErrKind` string vocabulary, scoped to `internal/driver`, not
   reused from `model.ErrorKind`.** `internal/driver` cannot import
   `internal/model` (`TestNoWireImports`, S01's AC-05 enforcement) so it
   needs its own error-class constants. I'm introducing:
   `ErrKindConfig = "config"`, `ErrKindTransient = "transient"`,
   `ErrKindProvider = "provider"`, `ErrKindProtocol = "protocol"` in
   `subprocess.go`. These are a **new contract surface** beyond what S01
   shipped (S01 declared `Result.ErrKind string` but no constants) — flagging
   for design review since S03/S04 will presumably reuse this vocabulary.

2. **Error-kind mapping deliberately diverges from `cli.go`'s existing
   heuristic**, per AC-04's literal text: binary-not-found → `config`
   (cli.go used the coarser `KindOther`); non-zero exit → `provider` with a
   stderr excerpt (cli.go guessed `KindAuth` — assumed "not logged in").
   The new mapping is more honest: a non-zero exit is classified by what
   actually happened (a provider/CLI-side failure), not by guessing the
   cause; the stderr excerpt in the message carries the diagnostic detail
   instead of a wrong-if-not-auth `ErrKind`. Timeout → `transient` matches
   cli.go's existing choice.

3. **Env hygiene: `GOCACHE`/`GOMODCACHE` redirected outside the worktree;
   `HOME` is never touched.** Fixed cache dir:
   `filepath.Join(os.TempDir(), "sworn-driver-cache")`. This is the
   opposite of the in-process tool executor's `HOME=root` hygiene
   (`internal/agent/tools.go:323`) — deliberate, because claude-cli's own
   login credentials live under the real `$HOME`; redirecting it would
   break auth. Spec's own rationale flags this contrast explicitly.

4. **`Roles()` returns `{implementer, verifier}` only — not `captain`.**
   The spec's user outcome and in-scope list only name implementer/verifier;
   captain-role claude-cli dispatch isn't described anywhere in this spec.
   Adding it speculatively would be scope creep un-traceable to an AC.

5. **`--no-session-persistence` is verifier-only**, per AC-01 vs AC-03's
   literal argv lists (AC-01 enumerates `-p --output-format json --model
   <model>` with no persistence flag for implementer; AC-03 explicitly adds
   it for verifier, tied to Rule 7 fresh-context). I'm implementing exactly
   what each AC states rather than harmonizing the two invocations.

6. **JSON envelope parsing is defensive, per R-01's own mitigation text**:
   unknown fields ignored (encoding/json default); missing `usage` fields
   degrade to `InputTokens=0, OutputTokens=0, CostSource="unknown"` rather
   than an error. When `total_cost_usd`/`usage` are present, `CostSource =
   "provider-reported"`. When the envelope's `model` field is absent, `
   Result.ModelID` falls back to `DispatchInput.ModelID` (never left empty).
   An envelope that fails to parse as JSON *at all* (the outer
   `--output-format json` envelope, not just the verifier's inner result
   text) maps to `ErrKind=protocol` — extending AC-03's protocol-error
   principle to the outer envelope too, since no AC explicitly covers that
   case and fail-closed matches the rest of the spec's posture.

7. **`capabilities_test.go` (internal/model) is touched even though it is
   not in `spec.json`'s `touchpoints` list.** It's in the same package as
   `cli.go`/`cli_test.go` (which *are* touchpoints) and it currently asserts
   `cliDriver` returns `CapVerify|CapChat` — AC-06 requires that assertion
   to become `CapVerify` only, and AC-06's own text requires `go test
   ./internal/model/...` to pass. Not updating it means AC-06 cannot be
   satisfied. Flagging as a bounded, same-package, spec-implied addition —
   not scope creep — for the reviewer to confirm.

8. **`internal/model/registry.go`'s static `capabilityRegistry` entry for
   `"claude-cli"` is left unchanged** (still lists `CapVerify|CapChat`).
   That registry is explicitly out of scope here (spec: "Registry
   registration and prefix mapping (S05)"), and its own doc comment says
   the canonical capability check is the `CapabilityProvider` interface
   method, not this list. This creates a **temporary, bounded
   inconsistency** (a discoverability list says claude-cli can Chat; the
   actual driver no longer advertises it) until S05 rewires the registry.
   Confirmed the real enforcement point (`internal/run/run.go:353`'s
   `CapChat` gate on `newAgentFromModel`) reads the driver's own
   `Capabilities()`, not this list — so no functional gap, only a stale
   metadata row. Surfacing for the reviewer in case the bounded
   inconsistency is judged unacceptable even temporarily.

9. **Open design question for the reviewer (not blocking, not tested by
   this slice's ACs): does unattended `claude -p` need a permission/tool-
   approval flag** (e.g. `--dangerously-skip-permissions` or
   `--permission-mode`) for the implementer role's file edits/bash calls to
   proceed without an interactive approval prompt? AC-01 enumerates the
   exact argv (`-p --output-format json --model <model>`) with nothing
   else, and this slice's fake-binary tests only assert against that literal
   argv — so implementing exactly the AC's argv is correct *for this
   slice's acceptance checks*. But real-CLI integration proof is explicitly
   out of scope here (S10's SIT smoke + the Rule-10 cutover journey walk),
   which is exactly where a missing permission flag would first surface as
   a hang or a rejected edit. Flagging now, before that slice, rather than
   letting it surface as a surprise at SIT time.

## Files touched

| File | Change |
|---|---|
| `internal/driver/subprocess.go` | NEW — shared spawn/env-hygiene/error-classification plumbing + `ErrKind` constants |
| `internal/driver/subprocess_test.go` | NEW — fake-binary `TestMain` harness (re-exec pattern, same convention as `internal/model/cli_test.go`) + plumbing-level tests |
| `internal/driver/claude.go` | NEW — `ClaudeDriver` (`Name`, `Roles`, `Dispatch`), argv construction, JSON envelope parsing |
| `internal/driver/claude_test.go` | NEW — AC-01..AC-05 tests: `TestClaudeDispatchImplementer`, `TestClaudeDispatchVerifier` (AC-03), `TestClaudeWorktreeGate` (AC-02), `TestClaudeErrorMapping` (AC-04, table-driven), `TestClaudeEnvHygiene` (AC-05) |
| `internal/model/cli.go` | Remove `CapChat` from `Capabilities()`; delete the toolless `Chat` method. `Verify` unchanged. |
| `internal/model/cli_test.go` | Remove/adjust any assertions tied to `cliDriver.Chat` (none currently call it directly — confirmed by grep; existing `Verify`-path tests are unaffected) |
| `internal/model/capabilities_test.go` | Move `cliDriver` from the Chat-capable list to the no-Chat list in both `TestCapabilities_AllDrivers` and `TestCapabilities_ChatBit` |

## AC traceability

| AC | Covered by |
|---|---|
| AC-01 | `claude.go` Dispatch (implementer path) + `TestClaudeDispatchImplementer` |
| AC-02 | `claude.go` Dispatch calling `AssertWorktree` before spawn + `TestClaudeWorktreeGate` |
| AC-03 | `claude.go` Dispatch (verifier path: `--no-session-persistence`, `VerdictSchema` in prompt, `StructuredJSON` / `ErrKind=protocol`) + `TestClaudeDispatchVerifier` |
| AC-04 | `subprocess.go` error classification + `TestClaudeErrorMapping` (table-driven: timeout/missing-binary/non-zero-exit) |
| AC-05 | `subprocess.go` env hygiene helper + `TestClaudeEnvHygiene` |
| AC-06 | `cli.go` Capabilities/Chat removal + `capabilities_test.go` updates; `go test ./internal/driver/... ./internal/model/...` |

## Risks / open items for the Captain

- Item 9 above (permission-flag question) — informational, not blocking;
  owned by S10's SIT smoke per spec's own out-of-scope list.
- Item 8 above (registry.go temporary inconsistency) — accept or push back;
  either way it's bounded and resolves at S05.
- Item 1/2 above (new `ErrKind` vocabulary + divergent mapping from cli.go)
  is the one piece of this slice with the most "shape" for S03/S04 to
  inherit — worth a second pair of eyes since it's new contract surface,
  not just an internal implementation detail.
