# Design TL;DR: `S33-spec-template-hardening`

## §1. User-visible change

Three new guard-rail rules are added to the Planner role prompt (`internal/prompt/planner.md`) that guide spec authors to pre-empt defects before implementation: (a) every Risk mitigation must cite a live `file:line` code surface, (b) pure-engine Go slices must include a failing-test commit for a non-empty diff, and (c) UI slices must note that dynamic CORS supersedes static port allowlists. The `[[feedback_worktree_devserver_cors_port]]` memory is marked stale. Additionally, the WATCHER comment-wrapper is cleaned from the end-of-turn status block in the embedded prompt files — though investigation reveals it exists in `implementer.md` (not `verifier.md` as the spec claims). No Go code changes.

## §2. Design decisions not in spec (max 5)

1. **Rule placement within planner.md**: All three rules land in Phase 4 ("Write specs") of `planner.md` — this is where the spec-authoring guidance lives and where the planner reads it during spec creation. Rationale: earlier phases (discovery, decomposition) are structural; Phase 4 is the spec-content phase where Risks get written and slice type (UI vs engine) is known.

2. **Memory-staleness anchoring**: The `[[feedback_worktree_devserver_cors_port]]` staleness note is placed inline in the planner prompt as a comment adjacent to the dynamic-CORS rule. Since the planner prompt already references project memory (`~/.claude/projects/.../memory/`), adding the staleness note here ensures the planner sees it. No separate memory-file edit is made (that's a UX concern for the Coach's tooling, not a planner prompt concern).

3. **Task (d) WATCHER cleanup — no-op on verifier.md**: Verifier.md has no WATCHER block (confirmed via grep on worktree, release-wt, and the S04 commit that introduced it). The WATCHER block lives in `implementer.md` at line 183. The slash-command copy (`~/.claude/baton/role-prompts/verifier.md`) is also already clean. This means task (d) as specified is already satisfied for verifier.md — the file listed in `planned_files` will not be changed by this slice.

4. **Acceptance check coverage gap**: Task (d) is listed in "In scope" but has no corresponding acceptance check (the four ACs only cover tasks a-c and the Go build sanity). WATCHER cleanup in implementer.md is not in scope of this slice per the spec's planned touchpoints.

5. **No external template edit**: Confirmed the spec's touchpoint note — the real spec template at `$HOME/.claude/baton/release-mode-template/spec.md` is external to this repo. This slice lands rules only in `internal/prompt/planner.md`.

## §3. Files I'll touch grouped by purpose

- **`internal/prompt/planner.md`** — Add three Phase 4 spec-authoring rules: Risk-cites-code, shape-pin/two-commit, dynamic-CORS + memory-stale note. This is the single in-repo file where the Planner reads spec-authoring guidance.

## §4. Things I'm NOT doing

- NOT editing `internal/prompt/verifier.md` — it has no WATCHER block; the spec's task (d) is a no-op on this file.
- NOT editing `internal/prompt/implementer.md` — its WATCHER block at line 183 is outside this slice's planned touchpoints.
- NOT editing `$HOME/.claude/baton/release-mode-template/spec.md` — external to repo (Rule 2 deferral, surfaced in spec).
- NOT adding any Go code or mechanical enforcement.

## §5. Reachability plan

The user-reachable artefact is the Planner role prompt itself — the rules are read by the Planner during spec authoring. Proof: quote the three new rule blocks from `planner.md` in `proof.md`, plus `go build ./...` passes to confirm no incidental breakage.

## §6. Open questions for the Coach

1. **WATCHER cleanup location discrepancy**: The spec says to clean WATCHER from `verifier.md`, but the WATCHER block is in `implementer.md:183`. Verifier.md is clean (never had it). Should I also clean it from `implementer.md`, or is that a separate task? (The spec's `planned_files` lists only `planner.md` and `verifier.md` — implementer.md is not in scope.)
2. **Should the three rules also land in the external `$HOME/.claude/baton/release-mode-template/spec.md`?** Surfaced as a Rule-2 deferral candidate per spec.