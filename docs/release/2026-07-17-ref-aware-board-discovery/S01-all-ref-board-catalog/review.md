# Captain review — S01-all-ref-board-catalog
Date: 2026-07-17
Design commit: f2995e6bcbf918ddcc893e45f22e741ec5f7725c

## Pins

1. [mechanical] §2.PIN-1 — Keep legacy index parsing strictly read-only
   What I observed: The design says discovery will adapt a selected ref into the current `ReadBoard`/board-record path, but live `ReadBoard` falls back to `migrateFromIndex`, which writes `board.json`; that cannot be invoked by this read-only catalog path.
   What to ask the implementer: Extract or add an in-memory parser for selected `board.json`/legacy `index.md` bytes and prove the catalog never reaches `migrateFromIndex` or any write path.

2. [mechanical] §1.3 — Recompute the complete SliceState from elected evidence
   What I observed: The approach says it will "replace each topology slice's status projection with the central evidence winner" and "writes the elected state and provenance into `SliceState`". Live `SliceState` also derives owner, timestamp, blocked visibility/owner/reason, verification metadata, actionability, and track/release aggregates from status evidence.
   What to ask the implementer: Parse the winning candidate into the complete `SliceState`, then derive track/release aggregates from those winners; do not patch only `State`, `StateSource`, and `StateDurability`. Add a test where a later `verification.result == blocked` winner changes blocked metadata and aggregate state.

3. [memory-cited] §2.PIN-1 — One catalog authority matches the learned architecture prior
   What I observed: `DiscoverCatalog` is designed as the only exported discovery/election authority, with CLI and S02 as consumers rather than alternate scanners or rankers.
   What to ask the implementer: Preserve that boundary during implementation and add shape/consumer assertions that fail if a caller bypasses the central result.
   Citation (if [memory-cited]): [[project_architecture_review_commissioned]]

4. [memory-cited] §2.PIN-1 — Selected board parsing must retain strict canonical release shape
   What I observed: Reusing the existing board-record parser is consistent with the ratified object-only `release` field and avoids introducing a tolerant second reader.
   What to ask the implementer: Reuse `BoardRecord`'s strict object unmarshal for ref-backed `board.json`; do not add string-release tolerance in discovery.
   Citation (if [memory-cited]): [[project_board_v1_release_shape_skew]]

5. [memory-cited] §2.PIN-5 — Verification-blocked attention evidence is load-bearing
   What I observed: The design explicitly treats `verification.result == blocked` as attention evidence, addressing the known class where an implemented slice otherwise appears normally implemented.
   What to ask the implementer: Keep the raw verification result and blocked routing/violations from the elected candidate visible in the complete `SliceState`, with regression coverage.
   Citation (if [memory-cited]): [[project_oracle_blocked_invisible]]

Pins: 5 total — 2 [mechanical], 3 [memory-cited], 0 [escalate]
Critical pins (if any): 1, 2

## Summary

Pins: 5 total — 2 [mechanical], 3 [memory-cited], 0 [escalate]
Critical pins (if any): 1, 2

## Smaller flags (not pins, worth one-line acknowledgement)

Filter remote symbolic-HEAD aliases by their symbolic-ref identity rather than assuming every remote branch whose final component is `HEAD` is an alias; keep the behaviour explicit in the ref-enumeration tests.

The required `sworn llm-check --type design-review` was attempted after the pin-driven review but was unavailable because the configured model account returned a quota-exceeded error. No LLM-check findings were available to incorporate.

## Suggested acknowledgement reply

TL;DR The design is sound and can proceed with two critical seam guards applied inline. 5 pins + 1 flag:

1. **Keep legacy parsing read-only.** Parse selected `board.json` or legacy `index.md` bytes in memory; the catalog must never invoke `ReadBoard`'s `migrateFromIndex` write path.
2. **Recompute complete elected state.** Build the full `SliceState` and all aggregates from the winning status candidate, including blocked/verification/actionability fields, rather than patching only lifecycle state and provenance; cover a later verification-blocked winner.
3. **Preserve one authority.** Keep `DiscoverCatalog` as the only exported discovery/election result used by CLI and S02, with assertions against bypass.
4. **Preserve strict board shape.** Reuse strict `BoardRecord` object-form release parsing and add no tolerant string reader.
5. **Preserve blocked evidence.** Carry verification result, blocked routing, reason, and violations from the elected candidate into the complete state projection.

Flags (not pins): (a) identify remote symbolic-HEAD aliases explicitly and pin that behaviour in ref-enumeration tests.

§2 decisions PIN-1 through PIN-5 and CHOICE-A/B acknowledged, including [[project_architecture_review_commissioned]], [[project_board_v1_release_shape_skew]], and [[project_oracle_blocked_invisible]]. §6 contains no open questions and is acknowledged.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The design matches the ratified spec and Type-1 choices; all pins are unambiguous apply-inline seam guards backstopped by tests and verification.
-->
