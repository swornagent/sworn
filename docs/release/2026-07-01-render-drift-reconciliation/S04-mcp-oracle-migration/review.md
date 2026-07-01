# Captain review — S04-mcp-oracle-migration
Date: 2026-07-02
Design commit: c5b449b1aaf34cb82415a1998d6d5c843f518d84

## Pins
1. [memory-cited] §2.1 — Oracle entry point vs project_index_frontmatter_corruption_false_ready
   What I observed: The design chooses `board.ReadBoard` (working-tree `board.json`) for filesystem fallback paths instead of the regex frontmatter scrape (`ParseTracks`).
   What to ask the implementer: Acknowledge this choice and confirm it completely replaces the fragile regex frontmatter scrapers and protects against the silent empty parses described in project_index_frontmatter_corruption_false_ready.
   Citation: [[project_index_frontmatter_corruption_false_ready]]

2. [memory-cited] §2.2 — `tools_plan.go` write-back via `RenderToFile` vs project_newline_eating_edit_corruption
   What I observed: Decision 2 implements plan-mutation write-back via `WriteBoard` + `RenderToFile`. This completely removes the hand-rolled YAML/Markdown parsing and writing, which previously caused newline-eating corruptions in `index.md`.
   What to ask the implementer: Confirm that by using `RenderToFile`, the plan-mutation tool relies on the canonical, correct rendering path, entirely mitigating the newline-eating edit corruption risk.
   Citation: [[project_newline_eating_edit_corruption]]

3. [memory-cited] §2.3 — `not_delivered` reader resilience vs string/object drift
   What I observed: Decision 3 implements a reader for `not_delivered` items that tolerates both string and object shapes. This is highly robust because real-world `proof.json` files have been observed to drift from string arrays to structured objects.
   What to ask the implementer: Confirm that the reader parses the `.item` field when an object is present, and falls back to the raw string otherwise, preventing unmarshal failure on real data.
   Citation: [[project_board_v1_release_shape_skew]]

4. [mechanical] §2.2b — Record Decision 2 in `status.json` as Type-1 choice
   What I observed: Decision 2 (`tools_plan.go` write-back) is identified as Type-1 (high-stakes/architecturally-significant) in `design.md` §2. However, the slice's `status.json` has an empty `design_decisions` list, so this Type-1 decision is not recorded.
   What to ask the implementer: Record this decision in `status.json`'s `design_decisions` list with `stake_class: "Type-1"`, `architecturally_significant: true`, and the Coach's decision once approved, to satisfy Rule 9's design-fit gate.

5. [mechanical] §Risks — Clean up unused helper functions
   What I observed: The design notes that `extractViolations`, `extractFrontmatterBody`, and `extractReleaseWorktreePath` may become unused after the migration.
   What to ask the implementer: Confirm that any unused helpers are deleted from the codebase during this slice implementation to prevent dead code and "attractive nuisances".

## Summary
Pins: 5 total — 2 [mechanical], 3 [memory-cited], 0 [escalate]
Critical pins (if any): None

## Smaller flags (not pins, worth one-line acknowledgement)
- Sibling status.json effort_complexity values have been repaired on this track branch (forward-merged via replan commit c5b449b), ensuring `sworn designfit` and parser routines pass cleanly.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Design is highly robust, completely replacing fragile regex scrapes in favour of the board oracle. 5 pins + 0 flags:

1. **Oracle entry point vs frontmatter corruption.** Confirm `board.ReadBoard` completely replaces the fragile regex frontmatter scrapers (`ParseTracks`) and protects against the silent empty parses described in [[project_index_frontmatter_corruption_false_ready]].
2. **`tools_plan.go` write-back via `RenderToFile`.** Confirm that by using `RenderToFile`, the plan-mutation tool relies on the canonical, correct rendering path, entirely mitigating the [[project_newline_eating_edit_corruption]] risk.
3. **`not_delivered` reader resilience.** Confirm that the reader parses the `.item` field when an object is present, and falls back to the raw string otherwise, preventing unmarshal failure on real data.
4. **Record Type-1 decision.** Record Decision 2 in `status.json`'s `design_decisions` list with `stake_class: "Type-1"`, `architecturally_significant: true`, and the Coach's decision once approved, to satisfy Rule 9's design-fit gate.
5. **Clean up unused helper functions.** Ensure that any unused helper functions (`extractViolations`, `extractFrontmatterBody`, `extractReleaseWorktreePath`) are deleted from the codebase during this slice implementation.

§2 decisions (1-5) and design choices acknowledged. All project memories [[project_index_frontmatter_corruption_false_ready]], [[project_newline_eating_edit_corruption]], and [[project_board_v1_release_shape_skew]] cited correctly and confirmed.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: No escalations; design is highly robust and completely eliminates fragile regex scrapes in favour of the board oracle.
-->
