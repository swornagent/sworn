# Proof Bundle: `S01-verifier-core`

## Scope

A developer runs `sworn verify --spec <p> --diff <p>` and receives a fail-closed
JSON verdict (PASS/FAIL/BLOCKED); the process exits `0` only on PASS.

## Files changed

The S01 scaffold (`cmd/sworn/main.go`, `internal/verdict/verdict.go`,
`internal/model/client.go`, `internal/verify/verify.go`) was committed at
`253bc10` on `release/v0.1.0` before the release worktree was cut. This
session validates the scaffold against all four acceptance checks and adds
the Coach-requested missing-file test.

```
$ git diff --name-only 3a53437869f11102b5d4cd42eacda8fec49c005b
docs/release/2026-06-15-e2e-turnkey-loop/S01-verifier-core/status.json
```

Track-level additions vs `release-wt/2026-06-15-e2e-turnkey-loop`:

```
$ git diff release-wt/2026-06-15-e2e-turnkey-loop --name-only
docs/release/2026-06-15-e2e-turnkey-loop/S01-verifier-core/approved-ack.md
docs/release/2026-06-15-e2e-turnkey-loop/S01-verifier-core/design.md
docs/release/2026-06-15-e2e-turnkey-loop/S01-verifier-core/review.md
docs/release/2026-06-15-e2e-turnkey-loop/S01-verifier-core/status.json
internal/verify/verify_test.go
```

Scaffold code (committed to `release/v0.1.0` at `253bc10`, present on release-wt):

- `cmd/sworn/main.go`
- `internal/verdict/verdict.go`
- `internal/model/client.go`
- `internal/verify/verify.go`

Session change: added `TestRun_MissingFileBlocks` to `internal/verify/verify_test.go`
(Coach Flag (a): non-existent file path → `first_pass:spec` BLOCKED).

## Test results

### Go

```
$ go test ./... -v
?       github.com/swornagent/sworn/cmd/sworn  [no test files]
=== RUN   TestValidateIndex
--- PASS: TestValidateIndex (0.00s)
    --- PASS: TestValidateIndex/well-formed_release_board (0.00s)
    --- PASS: TestValidateIndex/capture_index_(no_tracks)_is_fine (0.00s)
    --- PASS: TestValidateIndex/missing_frontmatter (0.00s)
    --- PASS: TestValidateIndex/closing_---_grafted_onto_a_value_line (0.00s)
    --- PASS: TestValidateIndex/key_hidden_after_a_#_comment (0.00s)
    --- PASS: TestValidateIndex/list_item_grafted_onto_a_value_line_(lost_newline) (0.00s)
    --- PASS: TestValidateIndex/tracks_present_but_no_entries (0.00s)
    --- PASS: TestValidateIndex/track_missing_slices (0.00s)
    --- PASS: TestValidateIndex/track_missing_branch (0.00s)
    --- PASS: TestValidateIndex/duplicate_track_id (0.00s)
    --- PASS: TestValidateIndex/block-style_slices_and_legacy_branch:_pass (0.00s)
=== RUN   TestLiveReleaseBoardsAreValid
--- PASS: TestLiveReleaseBoardsAreValid (0.00s)
PASS
ok      github.com/swornagent/sworn/internal/board    0.003s
?       github.com/swornagent/sworn/internal/model     [no test files]
?       github.com/swornagent/sworn/internal/verdict   [no test files]
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
ok      github.com/swornagent/sworn/internal/verify    0.006s
```

```
$ go vet ./...
(clean, exit 0)
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **Path**: CLI binary at `bin/sworn` (built via `go build -o bin/sworn ./cmd/sworn/`)
- **User gesture**: Developer runs `sworn verify --spec <path> --diff <path>` and observes JSON verdict + correct exit codes.

**AC2 — Empty spec → BLOCKED (exit 2):**

```
$ ./bin/sworn verify --spec <(printf '   ') --diff <(echo '+x')
{
  "verdict": "BLOCKED",
  "failed_gate": "first_pass:spec",
  "rationale": "/dev/fd/63 is empty",
  "cost_usd": 0
}
EXIT=2
```

**AC2 — Missing spec file → BLOCKED (exit 2):**

```
$ ./bin/sworn verify --spec /tmp/no-such-file-12345.md --diff <(echo '+x')
{
  "verdict": "BLOCKED",
  "failed_gate": "first_pass:spec",
  "rationale": "open /tmp/no-such-file-12345.md: no such file or directory",
  "cost_usd": 0
}
EXIT=2
```

**AC3 — Unconfigured model → BLOCKED (exit 2):**

```
$ ./bin/sworn verify --spec <(echo 'must do X') --diff <(echo '+did X')
{
  "verdict": "BLOCKED",
  "failed_gate": "verifier_dispatch",
  "rationale": "verifier model not configured (pass --verifier-model and the provider key)",
  "cost_usd": 0
}
EXIT=2
```

**AC1 — PASS/FAIL verified via Go tests:** `TestRun_PassExitsZero` (PASS→0) and
`TestRun_GarbledVerdictBlocks` (garbled→BLOCKED, gap-closes FAIL). Real PASS/FAIL
CLI paths require a configured model (S02).

## Delivered

- **AC1: PASS→exit 0, FAIL→1, BLOCKED→2** — evidence: `internal/verdict/verdict.go` `ExitCode()` (lines 31-39), `TestRun_PassExitsZero` asserts PASS/0
- **AC2: Missing/empty spec or diff → BLOCKED (`first_pass:*`)** — evidence: `TestRun_MissingSpecBlocks` (empty content) + `TestRun_MissingFileBlocks` (non-existent path), CLI smoke above
- **AC3: Unconfigured model → BLOCKED (fail-closed)** — evidence: `TestRun_UnconfiguredModelFailsClosed`, CLI smoke above
- **AC4: Unparseable reply → BLOCKED (`unparseable_verdict`)** — evidence: `TestRun_GarbledVerdictBlocks`, `verify.go` `parseVerdict` lines 78-91

## Not delivered

None. All four acceptance checks are demonstrably satisfied.

## Divergence from plan

- **Forward-compatible `--proof` and `--verifier-model` flags** are pre-wired in
  `cmd/sworn/main.go`. These are not in the spec's In Scope for S01 (they belong
  to S02/S05). Coach acknowledged the pre-wiring is acceptable (Pin 2,
  `approved-ack.md`).
- **`internal/verify/verify_test.go`** was in `actual_files` but not
  `planned_files`. Corrected in this session (Flag (b)).

## First-pass script output

```
$ release-verify.sh S01-verifier-core 2026-06-15-e2e-turnkey-loop
release-verify.sh
  slice:       S01-verifier-core
  slice dir:   docs/release/2026-06-15-e2e-turnkey-loop/S01-verifier-core
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
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 3a53437869f11102b5d4cd42eacda8fec49c005b
  PASS  2 file(s) changed vs diff base
  (first 20)
    AGENTS.md
    docs/release/2026-06-15-e2e-turnkey-loop/S01-verifier-core/status.json

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
  PASS  proof.md 'Files changed' count (~6) consistent with diff vs start_commit (2)

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 22
  checks failed: 0

FIRST-PASS PASS
Open a FRESH session and paste role-prompts/verifier.md to perform adversarial verification.
Do NOT run the verifier in this same session — Rule 7 requires a fresh context window.
```