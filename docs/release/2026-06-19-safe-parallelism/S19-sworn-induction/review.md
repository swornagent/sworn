# Captain review — S19-sworn-induction
Date: 2026-06-23
Design commit: 283298d45711ca497fed4ce8b0f6914363c4f57d

## Pins

1. [mechanical] §risk3/§4 — `cmd/sworn/main.go` in status.json `planned_files` contradicts spec and design
   What I observed: `planned_files` in status.json includes `"cmd/sworn/main.go"`. Spec Risk 3 states explicitly "S19 must NOT edit it"; design §4 confirms "Not editing `cmd/sworn/main.go`". Having it in `planned_files` causes verifier Gate 2 (planned_files vs actual_files) to FAIL when main.go is absent from the diff — a guaranteed false FAIL at verification.
   What to ask the implementer: Remove `"cmd/sworn/main.go"` from `planned_files` in status.json before transitioning to `in_progress`.

2. [mechanical] §2b — `design_decisions` field absent from status.json
   What I observed: status.json has no `design_decisions` key. The designfit gate (`sworn designfit <release>`) reads this field to classify each §2 decision as Type-1 or Type-2. With the field absent, the gate trivially passes (no decisions to check), defeating its purpose. The five decisions in design.md §2 need classification here before code.
   What to ask the implementer: Add `design_decisions` to status.json, one entry per §2 decision, each with `type` (Type-1 or Type-2) and `choice` (the decision made). Decisions 1–3 and 5 are Type-2 (reversible implementation choices); Decision 4 (idempotent detection signal) is on the boundary — see Pin 3 for its resolution, then classify accordingly.

3. [mechanical] §2.Decision4 — idempotent mode trigger drifts from spec AC5
   What I observed: Design Decision 4: "If the considerations file exists and has a non-empty `design_system.location`, treat as `--update` mode." Spec AC5: "`sworn induction` on a repo where `docs/considerations.md` already has patterns auto-enters `--update` mode with a notice." A repo that answered 'n' to design system setup (leaving `design_system.location` empty) but accepted patterns would have an empty `location` field. Decision 4's trigger would re-run full induction instead of entering `--update` mode — AC5 fails.
   What to ask the implementer: Change the idempotent-mode trigger to check `architecture.patterns` is non-empty (or simply that `docs/considerations.md` exists with any content), not `design_system.location`. Confirm the new trigger satisfies AC5: "already has patterns → auto-enter `--update` mode."

4. [memory-cited] §2.Decision3 — no-YAML-library aligns with [[feedback_dep_justification_test]]
   What I observed: Design Decision 3 states "We don't use a YAML library for the frontmatter — stdlib string manipulation is sufficient for the simple list structures." The considerations.md format is sworn-authored and fixed-schema (pattern/location/intent), exactly matching the S08c precedent where yaml.v3 was rejected for the same reason.
   What to ask the implementer: Ack the memory citation — confirm that stdlib string manipulation for the frontmatter is the intended approach and no new dep is introduced.
   Citation: [[feedback_dep_justification_test]]

5. [mechanical] §5 — test_commands broad patterns pre-satisfy on existing tests
   What I observed: `test_commands` use `-run Implementer` and `-run Verifier`. Both currently match existing tests (`TestImplementer_NonEmpty`, `TestVerifier_NonEmpty`, etc.) that pass without S19's new tests existing. If `TestImplementerHasDeviationCheck`, `TestImplementerHasDependencyDiscipline`, and `TestVerifierHasCatalogConformance` are not added, the test commands still report green — same false-green pattern as S18 (trial log: "CRITICAL: both test_commands miss spec test names → false green at verify"). The verifier's Gate 3 checks for required test existence separately but test_commands should be discriminating.
   What to ask the implementer: Either tighten test_commands to `-run TestImplementerHasDeviationCheck\|TestImplementerHasDependencyDiscipline` and `-run TestVerifierHasCatalogConformance`, or ensure verifier Gate 3 explicitly names these three tests in proof.md's "Required tests exist" evidence. Adding a third test command `go test ./internal/prompt/... -run TestImplementerHasDeviationCheck` is the safer fix.

## Summary

Pins: 5 total — 4 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: Pin 1 (main.go in planned_files causes guaranteed Gate 2 FAIL at verification), Pin 3 (idempotent trigger causes AC5 to fail if implemented as written)

## Smaller flags (not pins, worth one-line ack)

(a) `internal/prompt/verifier.md` is also touched by S38-verifier-blocked-violations (T12, `planned`) and S33-spec-template-hardening (T12, `planned`) in a different track worktree. At merge time the second lander confines their hunk to their additions and re-runs prompt tests — no blocking issue now, but implementer should coordinate hunk placement to minimize conflict surface.

(b) `architecture.patterns` lives in the YAML frontmatter (between `---`), while `[dependencies].project_pinned` lives in the markdown body under `## [dependencies]`. Design Decision 3 calls both "marker sections" — they require different parse anchors. Implementer should account for the frontmatter boundary (first `---` to second `---`) when locating and writing `patterns:`, and use the `## [dependencies]` heading anchor for `project_pinned:`.

(c) `prompt_test.go` currently lacks the three new test functions. They must be added as part of this slice.

## Suggested ack reply

<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Solid design, clear scope. 5 pins to apply inline before code:

1. **main.go out of planned_files.** Remove `"cmd/sworn/main.go"` from `planned_files` in status.json — spec Risk 3 forbids editing it; having it there causes Gate 2 FAIL at verify.
2. **Add design_decisions to status.json.** Five entries, one per §2 decision, each with `type` (Type-2 for Decisions 1–3 and 5; type for Decision 4 after Pin 3 resolution) and `choice`.
3. **Fix idempotent trigger.** Decision 4: change update-mode detection from `design_system.location` non-empty to `architecture.patterns` non-empty (or file exists with content). This is what AC5 tests against.
4. **No YAML library — ack.** Decision 3 stdlib-only approach confirmed per [[feedback_dep_justification_test]] precedent (same call as S08c).
5. **Tighten test_commands.** Add `go test ./internal/prompt/... -run TestImplementerHasDeviationCheck` as a third prompt test command, or ensure proof.md explicitly names the three new test functions as evidence.

Flags (not pins): (a) verifier.md merge collision with T12 slices — scope your hunk to additions only; (b) frontmatter vs markdown-body parse boundary for `patterns:` vs `project_pinned:` — use different anchors for each; (c) add the three missing test functions before claiming done.

§2 Decision 3 [memory-cited: [[feedback_dep_justification_test]]] ack. §6 empty ack.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are unambiguous inline corrections (remove main.go from planned_files, add design_decisions, fix idempotent trigger to match AC5, ack memory citation, tighten test commands) — no design change required; Verifier backstops.
-->
