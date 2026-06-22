---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S33-spec-template-hardening`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to harvest fixes §3a #5, #7, #8 from the trial-log
analysis (`2026-06-21-captain-trial-log-harvest.md`). Three prompt/template-only rules,
no Go:
(a) **Risk-cites-code** (theme T-G): Risk mitigations repeatedly assert assertions/audits
against code surfaces that do not exist — evidence `S01-personal-tenants` (assert on a
non-existent kernel error), `S03-federated-signout` (AC6 unsatisfiable), `S09-handbook`
(wrong port in AC). Require every Risk mitigation to cite a live `file:line`.
(b) **Shape-pin / two-commit** (theme T-L): pure-engine Go slices BLOCK the Verifier on an
empty git diff — add a note to include a failing-test commit.
(c) **Dynamic-CORS dev-server note** (theme T-J): recurring smoke-port-outside-allowlist
misses across S07/S20/S27/S29 until dynamic CORS injection landed; mark the
`[[feedback_worktree_devserver_cors_port]]` memory stale ("dynamic CORS supersedes static
port allowlist").

**Rationale:** design these three classes out at authoring time (markdown/prompt) so the
mechanical lints (S29–S31) and the Verifier gate see fewer of them.

**Touchpoint correction (flagged):** the brief named
`internal/adopt/baton/release-mode-template/spec.md` — that path does **not** exist in this
repo (`internal/adopt/baton/` ships only `rules/`, `VERSION`, `README.md`). The real spec
template is external (`$HOME/.claude/baton/release-mode-template/spec.md`, ref
`planner.md:22`). Rules are landed in `internal/prompt/planner.md`; the external-template
gap is surfaced for human acknowledgement, not silently edited.

Placed in new track `T12-harness-hardening` (depends_on `T1-concurrency-core`). Touches
`internal/prompt/planner.md` only — disjoint from S35 (`captain.md` + rules) and the lint
slices.

## Open questions

- Should the three rules also land in the external/shipped `release-mode-template/spec.md`?
  Surfaced as a Rule-2 deferral candidate (see spec Deferrals section) for human decision.

## Deferrals surfaced

- External `$HOME/.claude/baton/release-mode-template/spec.md` edit: why = file lives
  outside this repo; tracking = this slice's spec; acknowledgement = flagged in the replan
  summary. Not actioned in this slice.

## 2026-07-03 — implemented (re-entry, design_review → in_progress → implemented)

Re-entered slice after Coach approval (`approved-ack.md`). Applied all 5 pins:

- **PIN 1**: No separate memory file — in-planner comment IS the staleness marking. Proceeded.
- **PIN 2 (a)**: Added `implementer.md` to planned_files, cleaned WATCHER at line 183.
  Renamed section from "Watcher status block (mandatory)" to "## Status block", removed
  `<!-- WATCHER` / `-->` wrapper, kept metadata content.
- **PIN 3 (a)**: External template deferred — inline note in planner.md only, Rule-2 gate
  closed. No edit to `$HOME/.claude/baton/release-mode-template/spec.md`.
- **PIN 4**: Removed `verifier.md` from planned_files (no WATCHER block — task (d) was a
  no-op there), added `implementer.md`.
- **PIN 5**: Added `touchpoints` entry to status.json recording planner.md overlap with
  S18 (T3-commercial); note "second-lander confines hunk".

**Implementation:**
- `internal/prompt/planner.md` — Added three Phase 4 spec-authoring rules (items 11-13):
  Rule 11 (Risk-cites-code), Rule 12 (Shape-pin/two-commit), Rule 13 (Dynamic-CORS +
  memory-stale note). All placed after item 10, before Phase 5.
- `internal/prompt/implementer.md` — Cleaned WATCHER wrapper at line 183, renamed
  heading to "## Status block".
- No Go code changes; `go build ./...` passes.

**Forward-merged** release-wt into track (1 unrelated T15 commit).

**Touchpoint collision with S18:** S18 adds Phase 2b (consideration catalog); S33 adds
Phase 4 rules. Different sections, no same-line conflict. Recorded in status.json
touchpoints.

**Skeptic panel:** skipped — runtime does not support subagent dispatch (direct API mode).

## Verifier verdicts received

None yet.