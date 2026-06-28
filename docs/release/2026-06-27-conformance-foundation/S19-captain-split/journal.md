# S19-captain-split — Journal

## 2026-07-25 — Implementation session

**State transition:** `planned` → `in_progress` → `implemented`

### Decisions

1. **Split approach — keep captain.md intact with header note.** The spec's Risks section advises: "Callers of `prompt.Captain()` must not break; the safest approach is to update captain.md to delegate rather than fully removing its content in this slice." Followed this guidance: captain.md retains all content with a split-notice header added at the top. The "release-level orchestrator" phrase on line 3 was replaced with language describing the Captain as the design-review gate, noting that workflow coordination is now performed by the Sworn engine.

2. **design-reviewer.md is a standalone, self-contained role prompt.** Extracted the design-review function (`/design-review` section, six-step review, output format, failure modes) from captain.md into a clean, self-contained role prompt. This is the canonical home for the design-review function going forward.

3. **orchestrator-notes.md is a reference doc, not a role prompt.** It explicitly states that the release-orchestrator function is realised by the Sworn deterministic Go engine, cross-references S18's `docs/baton/roles/orchestrator.md` and `docs/baton/decisions/orchestrator-model.md`, and explains the split for future implementers.

4. **No Go code changes.** The spec's `In scope` says "Update `internal/captain/` package (if it exists)". No such package exists — the prompt package is at `internal/prompt/` and `prompt.Captain()` continues to work without modification (it embeds `captain.md` which still contains design-review content). No `go:embed` directive changes needed since the new files are documentation artefacts, not runtime dependencies.

### Trade-offs

- The design-reviewer.md file is large (~280 lines) because it extracts the full six-step review function verbatim. This is intentional: the design reviewer needs the complete function to operate correctly.
- The orchestrator-notes.md is deliberately concise (~65 lines). It references S18's formal docs rather than duplicating them.

### Out of scope (from spec)

- Re-vendoring captain.md from canonical (S20)
- Changing the design-review logic itself
- Any changes to the production Go code paths
## 2026-06-28T00:02:13Z — Verifier verdict (PASS)

**Verdict:** PASS
**Verified against:** 589a233f47f0f5d0edbce78bb45dbc000c8ed86a
**Verifier session:** fresh, artefact-only

Gate walk:
1. User-reachable outcome exists — `design-reviewer.md`, `orchestrator-notes.md`, and updated `captain.md` are all real prompt artefacts; `prompt.Captain()` continues to work (tests pass).
2. Planned touchpoints match actual changed files — all three planned files are the only production files changed.
3. Required tests exist and exercise the integration point — 24/24 tests pass, including `TestCaptain_NonEmpty`, `TestCaptain_ResolveDirtyWorktree`, `TestCaptainKeepsRoleVocab`.
4. Reachability artefact proves the user path — all artefacts exist on disk, non-empty, with correct content.
5. No silent deferrals or placeholder logic — zero TODO/FIXME/deferred/placeholder hits.
6. Design conformance — non-UI project, auto-pass.
7. Claimed scope matches implemented scope — all five delivered items match spec acceptance checks with valid evidence references.
