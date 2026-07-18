# Captain review — S01-tui-bounded-navigation
Date: 2026-07-18T20:26:04+10:00
Design commit: 313febb1f568ab8e5554c4e74f736204e593c855

## Pins

1. [memory-cited] §2.PIN-2 — Keep bounded and unbounded discovery behind one ranking/election authority
   What I observed: The design says "one discovery core" and preserves complete `DiscoverCatalog` while adding bounded selection before object reads. That directly honours the project's learned failure pattern: duplicated authorities drift and diverge silently.
   What to ask the implementer: Keep release ranking, topology validation/election, and status election in one core used by both APIs; prove bounded and unbounded callers cannot acquire separate ranking or fail-closed semantics.
   Citation (if [memory-cited]): [[project_architecture_review_commissioned]]

2. [memory-cited] §2.PIN-4 — Preserve the accepted catalog snapshot as the refresh transaction's sole state authority
   What I observed: The design installs releases and board from one bounded result and restores selections by ID. Prior TUI work established `boardViewFromCatalog` as the pure refresh hydrator because `LoadBoardFromCatalog` decorates through `ActiveMerges` and therefore reads a second status epoch.
   What to ask the implementer: Extend the existing pure `boardViewFromCatalog` refresh path with generation+positive-limit identity; do not call `LoadBoardFromCatalog`, `ActiveMerges`, or a second discovery during background apply, and preserve presentation-only decorations from the prior board.
   Citation (if [memory-cited]): [[S03-tui-live-board-refresh Captain review]]

3. [mechanical] §4.AC-04/AC-05 — Add visible terminal-frame proof for the user-facing scrolling, resize, focus, and help changes
   What I observed: The AC traceability plan lists deterministic `Model.Update`/`Model.View` assertions, but no before/after visual artefact for the terminal UI change. Tests prove state and bounds; a rendered frame proves the operator-visible result.
   What to ask the implementer: Capture representative before/after terminal frames at normal and constrained heights, including both focused panes and the three release footer states, and commit them under `screenshots/S01-tui-bounded-navigation/` for the proof bundle.

Pins: 3 total — 1 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins (if any): none

## Summary

Pins: 3 total — 1 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins (if any): none

## Smaller flags (not pins, worth one-line acknowledgement)

- `sworn lint design` reports workflow-owned `design.md` and `status.json` as undeclared production touchpoints; this is a tooling false positive tracked in swornagent/sworn#126, not a slice-design defect.
- The required design-review LLM check passed with no findings.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR The design is spec-complete, follows every prescribed risk mitigation, and is sound to implement. 3 pins + 2 flags:

1. **Keep one discovery authority.** Implement bounded and unbounded discovery through the same release-ranking, topology-validation/election, and status-election core; add tests that prevent their semantics from drifting.
2. **Keep refresh hydration snapshot-pure.** Extend the existing `boardViewFromCatalog` path with generation+positive-limit identity; do not call `LoadBoardFromCatalog`, `ActiveMerges`, or a second discovery during background apply, and preserve presentation decorations from the prior board.
3. **Capture visible terminal proof.** Commit representative before/after frames under `screenshots/S01-tui-bounded-navigation/` at normal and constrained heights, covering both pane-focus states and `o older`, `loading older`, and `all releases loaded`.

Flags (not pins): (a) the design-fit lint's workflow-artefact false positive is tracked in swornagent/sworn#126 and does not broaden this slice; (b) the design-review LLM check passed with no findings.

§2 decisions PIN-1 through PIN-8 and all three human-recorded Type-2 choices acknowledged. §6 has no open question; the low-effort/high-complexity classification is acknowledged.

Address pins 1–3 inline during implementation, then proceed to in_progress.

## Triage verdict

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The design matches the spec and risk mitigations; all pins are apply-inline authority confirmations or visible-proof work.
-->
