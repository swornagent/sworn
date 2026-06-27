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