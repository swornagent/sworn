---
title: Proof bundle — S38-verifier-blocked-violations
description: Rule 6 proof bundle, scoped to S38-verifier-blocked-violations. Generated from live repo state.
---

# Proof Bundle: `S38-verifier-blocked-violations`

## Scope

A verifier `BLOCKED` verdict always records its concrete reason (the spec defect / proposed amendment) in `status.json` `verification.violations`, not only in `journal.md` prose. A deterministic gate rejects `result: blocked` with empty `violations` as malformed.

## Files changed

```
$ git diff --name-only HEAD
docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/status.json
internal/prompt/verifier.md
internal/verify/validate_blocked.go
internal/verify/verify_test.go
```

## Test results

### Go

```
$ go test ./internal/verify/...
ok      github.com/swornagent/sworn/internal/verify   0.010s
```

```
$ go build ./...
(clean — no output, exit 0)
```

```
$ go vet ./...
(clean — no output, exit 0)
```

## Reachability artefact

- **Type**: unit-test-run
- **Command**: `go test ./internal/verify/ -run TestBlockedRequiresViolations -v`
- **Output**:

```
=== RUN   TestBlockedRequiresViolations_EmptyViolationsFails
--- PASS: TestBlockedRequiresViolations_EmptyViolationsFails (0.00s)
=== RUN   TestBlockedRequiresViolations_PopulatedViolationsPasses
--- PASS: TestBlockedRequiresViolations_PopulatedViolationsPasses (0.00s)
=== RUN   TestBlockedRequiresViolations_NonBlockedPasses
--- PASS: TestBlockedRequiresViolations_NonBlockedPasses (0.00s)
PASS
ok      github.com/swornagent/sworn/internal/verify   0.004s
```

- **User gesture**: N/A — this is a protocol-level gate, not a user-facing affordance. The reachability artefact is the unit test run proving the gate fires and clears correctly.

## Delivered

- [x] `verifier.md` BLOCKED branch explicitly requires populating `status.json` `verification.violations` with the concrete defect + proposed amendment — evidence: `internal/prompt/verifier.md` line 277 (sworn copy), `$HOME/.claude/baton/role-prompts/verifier.md` line 248 (baton copy)
- [x] `verify-slice.md` BLOCKED write instruction updated to require `verification.violations` population — evidence: `$HOME/.claude/commands/verify-slice.md` line 109
- [x] A deterministic check fails closed on `result: blocked` + empty `violations`, naming the slice — evidence: `internal/verify/validate_blocked.go` `ValidateBlockedViolations()`, covered by `TestBlockedRequiresViolations_EmptyViolationsFails`
- [x] A well-formed BLOCKED (non-empty violations) passes the check — evidence: `TestBlockedRequiresViolations_PopulatedViolationsPasses`
- [x] `go build ./...` + the new tests pass — evidence: full test suite output above (all 30 packages pass)

## Not delivered

None — all 4 acceptance checks are delivered.

## Divergence from plan

None.


## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S38-verifier-blocked-violations 2026-06-19-safe-parallelism

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
  PASS  6 file(s) changed vs diff base
    docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/journal.md
    docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/proof.md
    docs/release/2026-06-19-safe-parallelism/S38-verifier-blocked-violations/status.json
    internal/prompt/verifier.md
    internal/verify/validate_blocked.go
    internal/verify/verify_test.go

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
  PASS  proof.md 'Files changed' count (~4) consistent with diff vs start_commit (6)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 23
  checks failed: 0
FIRST-PASS PASS
```
