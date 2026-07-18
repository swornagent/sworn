# Captain review — S03-tui-live-board-refresh
Date: 2026-07-18T13:58:09+10:00
Design commit: 0ddcb9d6bdd810715074b07f8ec6bba16fdb2d3f

## Pins

1. [mechanical] §2.PIN-2 — Keep refresh board hydration free of secondary status reads
   What I observed: The design requires "one catalog, one transition" and notes that `LoadBoardFromCatalog` reads active-merge decorations. Live code confirms that helper calls `ActiveMerges`, which queries SQLite, so using it on the refresh path would violate the spec's prohibition on a second catalog/status resolver and could mix snapshot epochs.
   What to ask the implementer: Build the replacement board from the accepted `CatalogRecord` through a pure hydration path, preserve the prior `MergeActive` and matching gate presentation state, and prove the refresh path never calls `LoadBoardFromCatalog`, `ActiveMerges`, `DiscoverCatalog`, or another resolver. If extracting a helper requires `internal/tui/board.go`, declare that touchpoint before editing it.

2. [mechanical] §1.4 — Make refresh failures visible in every active root view
   What I observed: The design promises to render refresh failures through the existing `Error: ` presentation while the chain remains active across releases, board, live, log, blocked, and settings. Live `Model.View` returns early for live/log/blocked/settings before the root error renderer, so merely adding a refresh-specific field would leave failures invisible in those states.
   What to ask the implementer: Centralise or consistently append root error rendering for every non-quit view, without clearing or masking unrelated `errMsg` state, and add deterministic coverage for a refresh failure delivered while an alternate view is active.

3. [memory-cited] §2.PIN-2 — Reaffirm the shared catalog as the sole list-and-board authority
   What I observed: PIN-2 aligns with the prior S02 decision that the TUI consumes one immutable `board.DiscoverCatalog` snapshot, does not recompute state independently, and rejects stale asynchronous state rather than mixing authorities.
   What to ask the implementer: Treat the S02 shared-catalog rule as binding: one discovery result owns both the releases aggregate and selected board replacement, with no TUI-local state election.
   Citation (if [memory-cited]): [[S02-tui-ref-aware-release-navigation was implemented and proof-gated]]

4. [mechanical] §4.AC-01/AC-04 — Add visible TUI reachability evidence, not only model assertions
   What I observed: The traceability plan is entirely message-driven tests for a user-visible monitoring change. Those tests prove state transitions, but no before/after terminal frame is planned to show that the open TUI actually renders the new slice and the deterministic `Error: ` recovery path.
   What to ask the implementer: Capture deterministic before/after TUI frames at a representative viewport (or equivalent committed terminal screenshots) under `screenshots/S03-tui-live-board-refresh/`, cite them in the proof bundle, and keep the integration tests as the behavioural reachability artefact.

## Summary

Pins: 4 total — 3 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): 1, 2

## Smaller flags (not pins, worth one-line acknowledgement)

- The current `sworn lint design` invocation reports `design.md` and `status.json` as unplanned touchpoints because the pre-implementation fallback base is `release-wt/<release>`; rerun from the implementation baseline and do not misclassify workflow artefacts as production touchpoints merely to silence the check.
- Keep the five-second completion-relative cadence as an injected/named constant in tests; the observed discovery duration is repository-dependent and is not a correctness invariant.
- S01 and S02 are verified and already ancestral to this track; their shared TUI files create no active-sibling collision. The required `sworn llm-check --type design-review` returned PASS with no findings.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR The serial refresh design is sound and can proceed with four apply-inline corrections. 4 pins + 3 flags:

1. **Use pure snapshot hydration.** Build the refreshed board only from the accepted `CatalogRecord`; do not call `LoadBoardFromCatalog`, `ActiveMerges`, `DiscoverCatalog`, or another resolver. Preserve prior merge and matching gate presentation state, and declare `internal/tui/board.go` first if helper extraction adds that touchpoint.
2. **Render refresh errors in every root view.** Ensure releases, board, live, log, blocked, and settings can all display the deterministic `Error: ` refresh failure without clearing unrelated errors; cover an alternate-view delivery in tests.
3. **Keep shared-catalog authority binding.** One accepted discovery result owns both releases and selected-board replacement; do not add TUI-local state election.
4. **Capture visible reachability.** Add deterministic before/after terminal frames under `screenshots/S03-tui-live-board-refresh/` and cite them in the proof alongside the message-driven integration tests.

Flags (not pins): (a) rerun design lint from the implementation baseline rather than adding workflow docs as production touchpoints; (b) keep the five-second cadence injected and non-semantic; (c) S01/S02 ancestry is satisfied and the design-review LLM check passed.

§2 decision PIN-2 and its shared-catalog memory citation acknowledged; PIN-1 and PIN-3 through PIN-6 are clean. §6 has no open questions and is acknowledged.

Address pins 1–4 inline during implementation, then proceed to in_progress.

## Triage verdict

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The design is sound; all four pins are unambiguous apply-inline corrections that the verifier can directly backstop.
-->
