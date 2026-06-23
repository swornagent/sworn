<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR clean leaf design, 2 mechanical housekeeping items before code:

1. **CheckDeps null-fallback ref construction.** Confirm the implementation derives `<release>` from `status.Release` (the field already in the status.json you open), constructing `"release-wt/" + status.Release` as the fallback when `start_commit` is empty. If you instead want the caller to pass the release name, add a `releaseName string` parameter to `CheckDeps` and update Decision 1 to say so.
2. **Populate `design_decisions` in status.json.** All 4 decisions are Type-2 (designfit won't fail), but populate the field before transitioning — follow S28's `{id, stake_class, choice}` shape.

Flags (not pins): (a) update `cmdLint`'s top-level usage string to include `deps` and reflect its `<slice-id> <release>` arg count; (b) S30/S31 will also modify `cmd/sworn/lint.go` — serial T12 implementer handles on landing, no action now.

§2 decisions (all 4) ack — all Type-2, no human decision required. §6 open questions: none.

Address pins 1–2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both pins are apply-inline mechanical confirmations; neither requires re-checking the design before code is safe. Coach ack 2026-06-21.
-->
