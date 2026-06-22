# Captain review — S21-canonical-baton
Date: 2026-06-23
Design commit: 785619a3a166a7b9b90baec24b4a3bd373042ec6

## Pins

1. [mechanical] §3.file — ADR filename conflict: `0005` is taken
   What I observed: Design §3 lists `docs/adr/0005-canonical-baton.md` (echoing spec). ADR 0005 is already occupied by `0005-tui-dep-bubbles.md` (T2, merged to release-wt). Index.md replan note from 2026-06-22 explicitly states: "S21 must pick the next free number at implement time (→0008, after this replan's 0006)."
   What to ask the implementer: Create `docs/adr/0008-canonical-baton.md` instead. Update `planned_files` to `0008-canonical-baton.md`. Record divergence from spec in proof.md (spec AC8 references "0005" — a pre-collision artefact, resolved here).

2. [mechanical] §3.file — `internal/prompt/baton/track-mode.md` pre-exists from S08c
   What I observed: Design §3 lists `track-mode.md` in `planned_files` but the "New — Embedded Baton protocol" section only enumerates five files (rules.md, session-discipline.md, brainstorm-patterns.md, README.md, VERSION.txt). S08c-mcp-plan-tools already created `internal/prompt/baton/track-mode.md` (confirmed in S08c actual_files). On forward-merge of release-wt, this file will be present before any S21 code runs. The design does not acknowledge its pre-existence.
   What to ask the implementer: Do not recreate or overwrite `track-mode.md`. Confirm the existing content matches expectation; mark it "pre-existing, no action" in proof.md. Extend the embed in prompt.go from `baton/track-mode.md` to `baton/*` — this is still needed and handles the pre-existing file without re-writing it.

3. [mechanical] §2b — `design_decisions` field absent from status.json
   What I observed: `status.json` has no `design_decisions` field. `sworn designfit` checks this field. Five decisions are documented in §2 and must be recorded in `design_decisions[]` before transitioning to in_progress.
   What to ask the implementer: Populate `design_decisions` in status.json with the five §2 decisions (all Type-2) before calling `sworn designfit`. Harness hygiene requirement, not a design change.

4. [mechanical] §3.test — AC7 test fixture uses wrong legacy-detection string
   What I observed: Spec AC7 says "legacy Baton-splice AGENTS.md (contains `<!-- baton:start -->`)" as the detection trigger. Design decision 3 correctly identifies the real marker as `## Engineering Process — Baton` (from `adopt.BatonSectionHeading` at `internal/adopt/adopt.go:20`). The string `<!-- baton:start -->` does not exist in the codebase. A test seeded with `<!-- baton:start -->` would pass without exercising the detection branch.
   What to ask the implementer: Seed `TestInitWarnsLegacyBaton`'s fixture with `## Engineering Process — Baton` (the real constant), not `<!-- baton:start -->`. Note the spec string mismatch as a divergence in proof.md.

5. [memory-cited] §2.decisions 1–5 — embed-foundation / transform-split alignment
   What I observed: Decisions 1–5 position S21 as the verbatim-copy embed foundation that T14/S48 (`sworn baton vendor`) will transform. Design correctly stops before the transform layer.
   What to ask the implementer: Ack confirms this split is load-bearing per the architecture memory. Do not attempt bash→sworn rewrites in S21 content — that is T14's job.
   Citation: [[project_baton_sworn_architecture]]

## Summary

Pins: 5 total — 4 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: 1 (wrong ADR filename → Gate 2 FAIL at verify), 3 (missing design_decisions → designfit gate blocks verify), 4 (wrong test seed → AC7 silently untested)

## Smaller flags (not pins, worth one-line ack)

(a) `session-discipline.md` and `brainstorm-patterns.md` are sourced from `~/.claude/baton/` (design §3). The spec says "canonical Baton sources" without specifying the path; `~/.claude/baton/` is a local install of uncertain freshness. Since the critical-path concern (stale seven-rule set) applies only to `rules.md` (spec explicitly directs using `internal/adopt/baton/rules/` for that), these non-rule docs can reasonably come from the local install. Flag it in proof.md if any content diverges from expectation.

(b) prompt.go package comment says "prompts are vendored verbatim from the open Baton protocol (`~/.claude/baton/role-prompts/`)." After S21 adds `Baton()` and `BatonAll()` functions, the comment should be updated to mention the new `baton/` subdirectory. Minor, apply inline.

## Suggested ack reply

<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Clean design — 5 pins, all mechanical/memory. 3 are critical (1 + 3 + 4):

1. **ADR filename.** Use `docs/adr/0008-canonical-baton.md` (not 0005 — taken by T2). Update `planned_files` in status.json. Note divergence in proof.md.
2. **track-mode.md pre-exists.** S08c already created `internal/prompt/baton/track-mode.md`. Do not recreate or overwrite. Mark as pre-existing in proof.md. The embed extension to `baton/*` in prompt.go still needs to happen (replacing the explicit `baton/track-mode.md` in the embed directive).
3. **Populate `design_decisions` in status.json.** Record the five §2 decisions (all Type-2) before calling `sworn designfit`.
4. **AC7 test seed.** Use `## Engineering Process — Baton` (the real `adopt.BatonSectionHeading` constant) as the legacy-marker fixture string in `TestInitWarnsLegacyBaton`, not `<!-- baton:start -->`.
5. **T14 boundary.** No bash→sworn transforms in this slice — confirm S21 content is verbatim from sources. Transform is T14/S48's job per [[project_baton_sworn_architecture]].

Flags: (a) session-discipline.md + brainstorm-patterns.md sourced from `~/.claude/baton/` — fine for non-rule docs, note in proof.md if content diverges; (b) update prompt.go package comment to mention baton/ subdir.

§2 decisions 1–5 (all Type-2) ack. §6 empty ack.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are apply-inline corrections (wrong ADR filename, pre-existing file acknowledgement, missing status.json field, test seed string, memory ack) — none requires re-checking the design concept before code is safe.
-->
