---
title: Slice proof bundle
description: Rule 6 proof bundle for S33-spec-template-hardening. Generated from live repo state.
---

# Proof Bundle: `S33-spec-template-hardening`

## Scope

A planner authoring a slice spec is guided by template/prompt rules that pre-empt three
recurring Captain-catch classes, so the defects are designed out rather than caught.

## Files changed

```
$ git diff --name-only 348f2d2a0f831983b43d0bee76d8046b69d493e4^..HEAD
docs/release/2026-06-19-safe-parallelism/S33-spec-template-hardening/status.json
internal/prompt/implementer.md
internal/prompt/planner.md
```

## Test results

### Go (build sanity)

```
$ go build ./...
exit: 0
```

### Doc-content checks (acceptance checks)

```
$ grep -n "Risk-cites-code\|Risk mitigation.*cite.*live.*code\|file:line" internal/prompt/planner.md
186:11. **Risk-cites-code rule (Rule 11).** Every Risk mitigation in `spec.md` §Risks must cite a live code surface — a file path and line range (`file:line` or `file:line-line`) that exists at the time the spec is authored.

$ grep -n "shape-pin\|two-commit\|failing-test commit\|pure-engine Go" internal/prompt/planner.md
187:12. **Shape-pin / two-commit note (Rule 12).** A pure-engine Go slice (...) **Every pure-engine Go slice must include a failing-test commit before the implementation commit.**

$ grep -n "dynamic.CORS\|port-deriver\|feedback_worktree_devserver_cors_port.*stale\|supersedes.*static.*port" internal/prompt/planner.md
188:13. **Dynamic-CORS dev-server note (Rule 13).** ...Mark the `[[feedback_worktree_devserver_cors_port]]` memory **stale**...
190:    > **Memory staleness note:** `[[feedback_worktree_devserver_cors_port]]` is marked stale as of this rule.

$ grep -c "WATCHER" internal/prompt/implementer.md
0
(exit code 1 from grep -c with 0 matches = WATCHER fully removed)

$ grep -n "## Status block" internal/prompt/implementer.md
178:## Status block
```

## Reachability artefact

- **Type**: `manual-smoke-step`
- **User gesture**: Read the three new rule blocks from `internal/prompt/planner.md` lines 186-191 (the rules are the user-reachable artefact for a prompt-only change). Confirm each rule is present and unambiguous.

The three rule blocks:

**Rule 11 (Risk-cites-code)** — `internal/prompt/planner.md:186`:
> Every Risk mitigation in `spec.md` §Risks must cite a live code surface — a file path and line range (`file:line` or `file:line-line`) that exists at the time the spec is authored.

**Rule 12 (Shape-pin / two-commit)** — `internal/prompt/planner.md:187`:
> Every pure-engine Go slice must include a failing-test commit before the implementation commit. The test must exercise the entry-point function, fail because the function does not exist yet, and be committed separately.

**Rule 13 (Dynamic-CORS)** — `internal/prompt/planner.md:188`:
> When authoring a UI slice spec, include a note in `spec.md` `Risks` or `Notes`: the dev server on a derived port (worktree) serves CORS headers dynamically via the port-deriver... Mark the `[[feedback_worktree_devserver_cors_port]]` memory **stale**.

## Delivered

- [x] `internal/prompt/planner.md` contains a rule requiring every Risk mitigation to cite a live code surface (`file:line`) — evidence: `internal/prompt/planner.md:186` (Rule 11: Risk-cites-code)
- [x] `internal/prompt/planner.md` contains a shape-pin / two-commit rule for pure-engine (non-UI) Go slices (failing-test commit → non-empty git diff for the Verifier gate) — evidence: `internal/prompt/planner.md:187` (Rule 12: Shape-pin / two-commit note)
- [x] `internal/prompt/planner.md` contains a dynamic-CORS dev-server note AND marks the `[[feedback_worktree_devserver_cors_port]]` memory stale ("dynamic CORS supersedes static port allowlist") — evidence: `internal/prompt/planner.md:188` (Rule 13: Dynamic-CORS dev-server note) + `internal/prompt/planner.md:190` (Memory staleness note)
- [x] no Go files changed; `go build ./...` still passes — evidence: zero `.go` files in diff, `go build ./...` exits 0
- [x] (d) WATCHER cleanup — `internal/prompt/implementer.md` WATCHER wrapper removed, section renamed to "## Status block" — evidence: `grep -c WATCHER internal/prompt/implementer.md` → 0; section title is "## Status block" at line 178

## Not delivered

- External `$HOME/.claude/baton/release-mode-template/spec.md` edit — **Why**: file lives outside this repo (baton harness path, not in-repo surface). **Tracking**: S33 spec Deferrals section, open_deferrals in status.json. **Acknowledged**: Coach, 2026-06-22 (approved-ack.md PIN 3a — external template deferred; inline note in planner.md only, Rule-2 gate closed).

## Divergence from plan

- `planned_files` updated per Coach directive (PIN 2a, PIN 4): removed `internal/prompt/verifier.md` (no WATCHER block — task (d) was a no-op there), added `internal/prompt/implementer.md` (WATCHER cleanup at line 183). Spec had listed only `planner.md` and `verifier.md`; the verifier.md removal and implementer.md addition were ratified by Coach in approved-ack.md.
- Task (d) WATCHER cleanup landed in `implementer.md` (not `verifier.md` as spec originally stated). Spec's touchpoint note flagged the path discrepancy; Captain confirmed WATCHER lives in implementer.md:183. Cleaned there per Coach PIN 2(a).

## First-pass script output

```
$ $HOME/.claude/bin/release-verify.sh S33-spec-template-hardening 2026-06-19-safe-parallelism

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: in_progress
  FAIL  state is 'in_progress' — slice not yet ready for verifier; complete implementation first

== Integration branch drift ==
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  1 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files
```

Note: the `state is 'in_progress'` FAIL is expected — the script only passes after the slice transitions to `implemented`. The `PLAYWRIGHT_OPTIN: unbound variable` at script end is a harness-level bug (not slice-related).