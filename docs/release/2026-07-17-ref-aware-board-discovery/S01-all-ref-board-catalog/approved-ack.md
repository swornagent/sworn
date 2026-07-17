TL;DR The design is sound and can proceed with two critical seam guards applied inline. 5 pins + 1 flag:

1. **Keep legacy parsing read-only.** Parse selected `board.json` or legacy `index.md` bytes in memory; the catalog must never invoke `ReadBoard`'s `migrateFromIndex` write path.
2. **Recompute complete elected state.** Build the full `SliceState` and all aggregates from the winning status candidate, including blocked/verification/actionability fields, rather than patching only lifecycle state and provenance; cover a later verification-blocked winner.
3. **Preserve one authority.** Keep `DiscoverCatalog` as the only exported discovery/election result used by CLI and S02, with assertions against bypass.
4. **Preserve strict board shape.** Reuse strict `BoardRecord` object-form release parsing and add no tolerant string reader.
5. **Preserve blocked evidence.** Carry verification result, blocked routing, reason, and violations from the elected candidate into the complete state projection.

Flags (not pins): (a) identify remote symbolic-HEAD aliases explicitly and pin that behaviour in ref-enumeration tests.

§2 decisions PIN-1 through PIN-5 and CHOICE-A/B acknowledged, including [[project_architecture_review_commissioned]], [[project_board_v1_release_shape_skew]], and [[project_oracle_blocked_invisible]]. §6 contains no open questions and is acknowledged.

The quota-exceeded result from the supplementary design-review LLM check is acknowledged as unavailable audit evidence, not a design defect or unresolved choice. The complete mechanical and memory-cited review supplies the design-gate evidence; mandatory fresh-context verification remains the fail-closed implementation backstop.

Address pins 1–5 inline during implementation, then proceed to in_progress.
