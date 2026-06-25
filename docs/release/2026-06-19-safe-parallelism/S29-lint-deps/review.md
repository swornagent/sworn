# Captain review — S29-lint-deps
Date: 2026-06-21
Design commit: ec394fffbe062d4c6adda89a7e23a4a5515f9bba

## Pins

1. [mechanical] §2.1 — `CheckDeps` null-fallback ref construction is underspecified
   What I observed: Decision 1 says "If `start_commit` is null, it will default to `release-wt/<release>`." The proposed signature `CheckDeps(sliceDir, baseRef string)` has no `releaseName` parameter. The function can derive `<release>` from `sliceDir/status.json`'s `"release"` field (which the `state.Read` call already materialises), but the design does not say so explicitly. If the caller is expected to pass the release name instead, the signature must grow a third argument.
   What to ask the implementer: Confirm `CheckDeps` reads `status.Release` from the `status.json` it already opens to construct the fallback `"release-wt/" + status.Release` — or, if the release name is passed by the caller, update the function signature and Decision 1 accordingly. Either form is fine; the design just needs to state it.

2. [mechanical] §2b (design-fit gate) — `design_decisions` field absent from status.json
   What I observed: `internal/state/state.go:141` defines `DesignDecisions []DesignDecision` as an optional JSON field. Design.md §2 records 4 decisions; status.json has no `design_decisions` array at all. `sworn designfit` will PASS (all 4 are Type-2, so no human-decision gate triggers), but the field should be populated for harness consistency — the trial log records this as the expected pattern (see S28 for the `{id, stake_class, choice}` shape). Populate before transitioning to `in_progress`.

---

Pins: 2 total — 2 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none — neither pin causes the slice to ship broken; both are apply-inline housekeeping.

## Summary

Clean, well-scoped design for a leaf lint command. Two mechanical housekeeping items before code: (1) make the null-fallback ref construction explicit in the function's implementation, and (2) populate `design_decisions` in status.json. No escalation needed.

## Smaller flags (not pins, worth one-line ack)

- **Usage string:** `cmdLint`'s top-level error message currently says `sworn lint <ac|trace> <release>`. Adding `deps` should update it to include `deps`, and note that `deps` takes `<slice-id> <release>` while `ac` and `trace` take only `<release>`. Minor UX detail not covered by an AC, but avoids confusing error messages.
- **S30/S31 also plan `cmd/sworn/lint.go`:** S30-lint-touchpoints and S31-lint-symbols both declare `cmd/sworn/lint.go` in their `planned_files`. Since all three are in the serial T12 worktree (one implementer), the second and third landers resolve merge conflicts on landing — no action needed now.
- **spec line reference:** Spec's Risks section cites `internal/state/state.go:141` for "status.json schema fields." Line 141 is `DesignDecisions`, not `StartCommit` (which is at line 118). Harmless but slightly misleading; no action required.

## Suggested ack reply
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
REASON: Both pins are apply-inline mechanical confirmations; neither requires re-checking the design before code is safe.
-->
