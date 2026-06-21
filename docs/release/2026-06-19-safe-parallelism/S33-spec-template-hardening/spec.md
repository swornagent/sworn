---
title: 'S33-spec-template-hardening — spec-template + planner prompt rules: Risk-cites-code, shape-pin note, dynamic-CORS note'
description: 'Markdown/prompt-only hardening of the spec template and planner prompt, harvesting three recurring Captain-catch classes: (a) Risk mitigations that assert code surfaces that do not exist (theme T-G) — require every Risk mitigation to cite a live code surface (file:line); (b) pure-engine Go slices that BLOCK the Verifier on an empty git diff (theme T-L) — add a two-commit / shape-pin note; (c) UI dev-server CORS-port misses (theme T-J) — add a dynamic-CORS note and mark the [[feedback_worktree_devserver_cors_port]] memory stale. No Go. Harvested from §3a #5, #7, #8.'
---

# Slice: `S33-spec-template-hardening`

## User outcome

A planner authoring a slice spec is guided by template/prompt rules that pre-empt three
recurring Captain-catch classes, so the defects are designed out rather than caught:

1. Every **Risk mitigation** must cite a live code surface (`file:line`) — no more Risk
   sections asserting an assertion/audit against a function, error, or anchor that does
   not exist (theme T-G).
2. Pure-engine (non-UI) Go slices include a **failing-test (shape-pin) commit** so the
   git diff is non-empty for the Verifier gate (theme T-L) — no more engine slices that
   BLOCK verification on an empty diff.
3. A **dynamic-CORS dev-server note** for UI slices, and the
   `[[feedback_worktree_devserver_cors_port]]` memory marked **stale** ("dynamic CORS
   supersedes static port allowlist") — closing the recurring smoke-port-outside-allowlist
   reachability miss (theme T-J).

## Entry point

The spec-authoring guidance read by the Planner role. In this repo the in-tree surface is
`internal/prompt/planner.md` (the Planner role prompt that drives spec authoring). The
canonical `spec.md` **template file** is referenced by `planner.md:22` /`:140` as living at
`$HOME/.claude/baton/release-mode-template/spec.md` — an external harness path, NOT in this
repo. Verifiable by: reading `internal/prompt/planner.md` and confirming the three new
checklist rules are present and unambiguous.

## In scope

- **(a) Risk-cites-code rule** — add a Planner checklist line: every Risk mitigation must
  name a live code surface (`file:line`). Lands in `internal/prompt/planner.md` (Phase 4
  spec-authoring guidance).
- **(b) Shape-pin / two-commit note** — add a rule for pure-engine Go slices: include a
  failing-test commit so `git diff` is non-empty for the Verifier gate. Lands in
  `internal/prompt/planner.md`.
- **(c) Dynamic-CORS dev-server note** — add a UI-slice note that dynamic CORS injection
  supersedes a static smoke-port allowlist, and mark the
  `[[feedback_worktree_devserver_cors_port]]` memory **stale**. The note lands in
  `internal/prompt/planner.md`; the memory-staleness is recorded where the planner reads it.

### (d) Remove the vestigial watcher comment-wrapper from the verifier status block

`internal/prompt/verifier.md` (the embedded copy) emits its end-of-turn status block wrapped in `<!-- WATCHER ... -->`, for a defunct watcher automation. Verified 2026-06-21 that nothing parses it (the live router `captain-route.sh` reads `status.json`; the word `WATCHER` appears in zero harness scripts). Keep the `STATE/SLICE/NEXT/REASON` metadata (useful human-readable state/resolution), drop the `<!-- WATCHER`/`-->` wrapper, and rename the section to `## Status block`. The live slash-command copy (`~/.claude/baton/role-prompts/verifier.md`) was already cleaned; this brings the embedded binary copy into parity.

## Out of scope

- Any Go code change — this slice is markdown/prompt only.
- Editing the external `$HOME/.claude/baton/release-mode-template/spec.md` file (outside
  this repo; see Risks — if the in-repo template surface is confirmed to also need the
  rule, that edit is surfaced as a separate acknowledged touchpoint, not assumed here).
- Enforcing the rules mechanically (that is the `sworn lint` family, S29–S31).

## Planned touchpoints

- `internal/prompt/planner.md` (add the three rules: Risk-cites-code, shape-pin/two-commit,
  dynamic-CORS + memory-stale note)

> **Touchpoint note (FLAG FOR HUMAN):** the task brief named
> `internal/adopt/baton/release-mode-template/spec.md` as the spec-template touchpoint.
> **That path does not exist in this repo** — `internal/adopt/baton/` ships only `rules/`,
> `VERSION`, `README.md`. The Planner role prompt (`planner.md`) references the spec
> template at the external `$HOME/.claude/baton/release-mode-template/spec.md`. This slice
> therefore lands the rules in `internal/prompt/planner.md` (the in-repo planner surface).
> If the rules must also live in the shipped/external template file, add that as a second
> acknowledged touchpoint before implementation.

## Acceptance checks

- [ ] `internal/prompt/planner.md` contains a rule requiring every Risk mitigation to cite
  a live code surface (`file:line`)
- [ ] `internal/prompt/planner.md` contains a shape-pin / two-commit rule for pure-engine
  (non-UI) Go slices (failing-test commit → non-empty git diff for the Verifier gate)
- [ ] `internal/prompt/planner.md` contains a dynamic-CORS dev-server note AND marks the
  `[[feedback_worktree_devserver_cors_port]]` memory stale ("dynamic CORS supersedes static
  port allowlist")
- [ ] no Go files changed; `go build ./...` still passes (sanity — no regression from a
  prompt-only change)

## Required tests

- This is a markdown/prompt-only slice; the "test" is a doc-content assertion plus a build
  sanity check.
- **Doc-content check**: grep `internal/prompt/planner.md` for each of the three new rules
  (Risk-cites-code, shape-pin, dynamic-CORS/memory-stale) and confirm presence.
- **Reachability artefact**: quote the three new rule blocks from `internal/prompt/planner.md`
  in `proof.md` (the rules are the user-reachable artefact for a prompt change); run
  `go build ./...` to confirm no incidental Go breakage. Document both in `proof.md`.

## Risks

- The real spec-template file is external to this repo (`$HOME/.claude/baton/release-mode-template/spec.md`,
  referenced at `internal/prompt/planner.md:22`), so a rule landed only in `planner.md` may
  not reach the shipped template. Mitigation: land the rules in the in-repo planner surface
  (`internal/prompt/planner.md`) where the Planner role actually reads them, and explicitly
  flag (above) the external-template gap for human acknowledgement rather than silently
  editing an out-of-repo file.

## Deferrals allowed?

The external-template edit is surfaced (Touchpoint note + Risks) as a Rule-2 deferral
candidate — why (file lives outside this repo), tracking (this spec), acknowledgement (flagged
to the human in the replan summary). No silent inline deferrals.
