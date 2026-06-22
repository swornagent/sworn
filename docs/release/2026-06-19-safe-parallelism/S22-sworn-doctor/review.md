# Captain review — S22-sworn-doctor
Date: 2026-06-21T00:00:00Z
Design commit: N/A

## Pins
1. [escalate] §2.1 — Design assumes multiple rule files (`internal/adopt/baton/rules/01-*.md` etc.) instead of the single `baton/rules.md` file referenced in the spec. This deviates from the spec's artifact expectations and may cause mismatched documentation or user confusion.
   What I observed: design decision "Spec references "baton/rules.md" as a single file — actual structure has separate rule files..."
   What to ask the implementer: confirm whether the spec should be updated to reflect the multi‑file structure or adjust the implementation to provide a combined `baton/rules.md` wrapper.

2. [escalate] §6.1 — The spec's group 1 says `baton/rules.md` must contain all 7 rule headings, but the actual embed structure has 10 separate rule files. Should doctor check for 7 or 10 rules?
   What I observed: design question about rule count mismatch.
   What to ask the implementer: decide which rule count is authoritative; if 10, update spec and acceptance checks accordingly.

3. [escalate] §6.2 — The spec's group 1 says planner.md must contain `## Phase 1` through `## Phase 4`, but the actual planner.md uses `### Phase 1` (h3). Should doctor check for `## Phase` or `### Phase`?
   What I observed: design question about heading level mismatch.
   What to ask the implementer: align spec heading levels with actual files or adjust doctor checks.

## Summary
Pins: 3 total — 0 [mechanical], 0 [memory-cited], 3 [escalate]
Critical pins (if any): 1, 2, 3

## Suggested ack reply
TL;DR Needs Coach review. Please address the escalated pins.

1. **Multiple rule files vs single `baton/rules.md`.** Clarify spec or implementation.
2. **Rule count 7 vs 10.** Decide authoritative count.
3. **Heading level `##` vs `###`.** Align spec with actual headings.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Design deviates from spec on baton rule files and heading levels; open questions require product decision.
-->