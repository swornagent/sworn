---
title: 'S19 — Split captain.md into design-reviewer.md and orchestrator-notes.md'
description: 'Split the captain.md artefact: the Rule-9 design-reviewer function becomes design-reviewer.md (owned by Baton); the release-orchestrator function is documented as orchestrator-notes.md (noting it is realised by the Sworn deterministic engine from S18).'
---

# Slice: `S19-captain-split`

## User outcome

`internal/prompt/captain.md` no longer conflates two identities. The design-review function is isolated in `internal/prompt/design-reviewer.md` (or equivalent). `internal/prompt/orchestrator-notes.md` documents that the "captain as release orchestrator" described in the prompt is realised by the Sworn engine, not a human-facing role prompt. Existing callers of `prompt.Captain()` continue to work (captain.md remains as a backward-compatible redirect or the callers are updated to use the new files).

## Entry point

`internal/prompt/captain.md` — split into two files.

## In scope

- `internal/prompt/design-reviewer.md` (new or rename from captain.md): contains the Rule-9 design-review function (Step 2a/2b, pin surfacing logic, stakes classification); this is the Baton role prompt for the Captain in its design-reviewer capacity
- `internal/prompt/orchestrator-notes.md` (new): documents that the release-orchestrator function in the original captain.md (routing, scheduling, managing the release loop) is realised by the Sworn deterministic engine (`internal/scheduler/`, `internal/orchestrator/`); not a prompt — a reference doc that cross-links to S18's orchestrator-formalized docs
- Update `internal/captain/` package (if it exists): update `prompt.Captain()` to serve `design-reviewer.md` content; add documentation that the orchestrator function is engine-side
- `internal/prompt/captain.md`: either (a) updated to clearly delegate to design-reviewer.md and note the split, or (b) kept as is with a header note "This file is being split — see design-reviewer.md and orchestrator-notes.md"
- The design-review gate in the engine (`internal/captain/` or wherever `prompt.Captain()` is served) must continue to work without change in behaviour

## Out of scope

- Re-vendoring captain.md from canonical (S20)
- Changing the design-review logic itself
- Any changes to the production Go code paths (the split is artefact/documentation only)

## Planned touchpoints

- `internal/prompt/design-reviewer.md` (new)
- `internal/prompt/orchestrator-notes.md` (new)
- `internal/prompt/captain.md` (add delegation header or minimal update)

## Acceptance checks

- [ ] `internal/prompt/design-reviewer.md` exists and contains the design-review role content (Step 2a/2b, pin surfacing, stakes classification)
- [ ] `internal/prompt/orchestrator-notes.md` exists and explicitly states that the release-orchestrator function is realised by the Sworn engine (not a prompt for a human or an LLM agent)
- [ ] `internal/prompt/captain.md` contains a header or first section noting the split (e.g. "The design-review function is in design-reviewer.md; the orchestrator function is realised by the Sworn engine — see orchestrator-notes.md")
- [ ] WHEN `prompt.Captain()` (or the MCP resource for captain) is called, THE SYSTEM SHALL return content that includes the design-review function (either from the updated captain.md or from design-reviewer.md)
- [ ] `grep -n "release orchestrator" internal/prompt/captain.md` — the original conflating language is either removed or annotated

## Required tests

- **Reachability artefact**: `cat internal/prompt/design-reviewer.md` exists and non-empty; `cat internal/prompt/orchestrator-notes.md` exists and non-empty; `sworn mcp` or the captain prompt resource returns content that includes design-review content

## Risks

- Callers of `prompt.Captain()` must not break; the safest approach is to update captain.md to delegate rather than fully removing its content in this slice

## Deferrals allowed?

No.
