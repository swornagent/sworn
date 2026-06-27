# Proof Bundle: `S04-embed-baton-prompts`

## Scope

The planner/implementer/verifier role prompts are embedded in the `sworn` binary
(from the open Baton protocol), so the loop runs with no external prompt files —
the inline placeholder in `verify` is replaced by the real Baton verifier prompt.

## Files changed

```
$ git diff --name-only 5396772ea000b501b2a48e1146e1aaf77d36a109
cmd/sworn/main.go
docs/release/2026-06-15-e2e-turnkey-loop/S04-embed-baton-prompts/journal.md
docs/release/2026-06-15-e2e-turnkey-loop/S04-embed-baton-prompts/proof.md
docs/release/2026-06-15-e2e-turnkey-loop/S04-embed-baton-prompts/status.json
internal/prompt/VERSION.txt
internal/prompt/captain.md
internal/prompt/implementer.md
internal/prompt/planner.md
internal/prompt/prompt.go
internal/prompt/prompt_test.go
internal/prompt/verifier.md
internal/verdict/verdict.go
internal/verify/verify.go
```
## Test results

### Go

```
$ go test ./... -v -count=1
?       github.com/swornagent/sworn/cmd/sworn    [no test files]
=== RUN   TestRun_SuccessPath
--- PASS: TestRun_SuccessPath (0.00s)
=== RUN   TestRun_ToolError_ModelAdapts
--- PASS: TestRun_ToolError_ModelAdapts (0.00s)
=== RUN   TestRun_TurnCap
--- PASS: TestRun_TurnCap (0.00s)
=== RUN   TestRun_WorkspaceConfinement
--- PASS: TestRun_WorkspaceConfinement (0.00s)
=== RUN   TestRun_PathTraversalRejected
--- PASS: TestRun_PathTraversalRejected (0.00s)
PASS
ok      github.com/swornagent/sworn/internal/agent        0.011s
=== RUN   TestValidateIndex
--- PASS: TestValidateIndex (0.00s)
PASS
ok      github.com/swornagent/sworn/internal/board        0.004s
=== RUN   TestOAI_Verify
--- PASS: TestOAI_Verify (0.20s)
PASS
ok      github.com/swornagent/sworn/internal/model        0.214s
=== RUN   TestVerifier_NonEmpty
--- PASS: TestVerifier_NonEmpty (0.00s)
=== RUN   TestVerifier_ContainsVerdictContract
--- PASS: TestVerifier_ContainsVerdictContract (0.00s)
=== RUN   TestVerifier_NotOldPlaceholder
--- PASS: TestVerifier_NotOldPlaceholder (0.00s)
=== RUN   TestVerifier_ContainsInconclusive
--- PASS: TestVerifier_ContainsInconclusive (0.00s)
=== RUN   TestImplementer_NonEmpty
--- PASS: TestImplementer_NonEmpty (0.00s)
=== RUN   TestPlanner_NonEmpty
--- PASS: TestPlanner_NonEmpty (0.00s)
=== RUN   TestCaptain_NonEmpty
--- PASS: TestCaptain_NonEmpty (0.00s)
=== RUN   TestBatonVersion_NonEmpty
--- PASS: TestBatonVersion_NonEmpty (0.00s)
PASS
ok      github.com/swornagent/sworn/internal/prompt       0.004s
?       github.com/swornagent/sworn/internal/verdict      [no test files]
=== RUN   TestRun_PassExitsZero
--- PASS: TestRun_PassExitsZero (0.00s)
=== RUN   TestRun_MissingSpecBlocks
--- PASS: TestRun_MissingSpecBlocks (0.00s)
=== RUN   TestRun_UnconfiguredModelFailsClosed
--- PASS: TestRun_UnconfiguredModelFailsClosed (0.00s)
=== RUN   TestRun_MissingFileBlocks
--- PASS: TestRun_MissingFileBlocks (0.00s)
=== RUN   TestRun_GarbledVerdictBlocks
--- PASS: TestRun_GarbledVerdictBlocks (0.00s)
PASS
ok      github.com/swornagent/sworn/internal/verify       0.006s
```

### go vet

```
$ go vet ./...
(no output — clean)
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: build artifact at `/tmp/sworn`
- **User gesture**: `go build -o /tmp/sworn ./cmd/sworn && /tmp/sworn version`

Output:
```
sworn 0.0.0-dev
baton-protocol v1.0.0
```

The binary prints both the sworn version and the vendored baton-protocol version
without reading any external prompt files at runtime — the prompt content is
embedded at build time via `go:embed`.

## Delivered

- [x] **AC1 — Verifier uses embedded Baton verifier prompt (not the placeholder).**
  Evidence: `internal/verify/verify.go` line 19 — `var systemPrompt = prompt.Verifier()` replaces the old `const systemPrompt`. `internal/prompt/prompt_test.go` `TestVerifier_NotOldPlaceholder` asserts the embedded prompt ≠ the old placeholder. `TestVerifier_ContainsVerdictContract` confirms PASS/FAIL/BLOCKED tokens present.

- [x] **AC2 — Build embeds the files (no runtime file read for prompts).**
  Evidence: `internal/prompt/prompt.go` uses `//go:embed verifier.md implementer.md planner.md captain.md VERSION.txt` — all prompt content is embedded at compile time. The binary at `/tmp/sworn` runs `version` successfully from any directory with no access to the source prompts. `TestVerifier_NonEmpty`, `TestImplementer_NonEmpty`, `TestPlanner_NonEmpty`, `TestCaptain_NonEmpty` all pass.

- [x] **AC3 — Vendored Baton version recorded and surfaced (`sworn version`).**
  Evidence: `internal/prompt/VERSION.txt` records `v1.0.0` with a bump-instruction comment. `internal/prompt/prompt.go` `BatonVersion()` returns it. `cmd/sworn/main.go` prints `baton-protocol v1.0.0` on `sworn version`. Binary smoke test confirms output: `baton-protocol v1.0.0`.

- [x] **Coach Pin 1 — INCONCLUSIVE verdict added.**
  Evidence: `internal/verdict/verdict.go` defines `Inconclusive Verdict = "INCONCLUSIVE"` with exit code 3. `internal/verify/verify.go` `parseVerdict` handles `INCONCLUSIVE` prefix, mapping to `verdict.Inconclusive` with `FailedGate: "adversarial"`.

- [x] **Coach Pin 2 — Memory ack (Baton protocol alignment).**
  Acknowledged. All four prompts vendored verbatim from `~/.claude/baton/role-prompts/` (open Baton protocol, MIT-licensed). The captain prompt's "S21 stall, 2026-05-30" reference is generic enough for open-source — no scrubbing needed per Coach.

- [x] **Coach Pin 3 — VERSION.txt bump tracking.**
  Evidence: `internal/prompt/VERSION.txt` opens with `# Bump this version whenever prompt files are re-vendored from upstream Baton`.

- [x] **Coach Pin 4 — Negative check in prompt test.**
  Evidence: `internal/prompt/prompt_test.go` `TestVerifier_NotOldPlaceholder` asserts embedded prompt ≠ old placeholder. `TestVerifier_ContainsInconclusive` asserts the embedded prompt contains `INCONCLUSIVE` (a token the placeholder lacks).

## Not delivered

- None. All four acceptance checks and all four Coach pins delivered.

## Divergence from plan

- **`internal/verdict/verdict.go`** — added INCONCLUSIVE verdict (not in original spec but required by Coach Pin 1; the embedded verifier prompt supports it, and S04 owns the prompt→parser boundary).
- **`cmd/sworn/main.go`** — added `github.com/swornagent/sworn/internal/prompt` import (consumed by the version line); additive change on a documented shared file — S02's `verify` case is untouched.
- **`docs/release/2026-06-15-e2e-turnkey-loop/S04-embed-baton-prompts/status.json`** — state transitions + start_commit recording (harness artefact, not production scope).

## First-pass script output

```$ release-verify.sh S04-embed-baton-prompts 2026-06-15-e2e-turnkey-loop

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
  PASS  13 file(s) changed vs diff base

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
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count (~11) consistent with diff vs start_commit (13)

== First-pass verdict ==
  checks passed: 22
  checks failed: 0

FIRST-PASS PASS
```
