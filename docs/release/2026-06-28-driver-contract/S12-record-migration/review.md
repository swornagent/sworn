# Captain review — S12-record-migration
Date: 2026-07-11
Design commit: ca98be2fb6de39b9c312b0c5821b2b4cb04bd89c

## Pins

1. [escalate] §3a/§6.P-2 — Stripping `ears_keyword` silently regresses `sworn lint`'s EARS classification (CRITICAL)
   What I observed: §3a strips the AC-item keys with `.acceptance_criteria |= map(del(.type, .ears_keyword))`, and P-2 justifies it as "no Go caller ever normalises spec-v1 (dead branch)". The `Normalise("spec-v1", …)` branch is indeed dead — verified: the only `Normalise` call sites are `board-v1` (board.go:137) and `slice-status-v1` (state.go:529), never `spec-v1`. BUT "not normalised" is not "not read." `internal/spec/spec.go` AC struct has `EARSKeyword string \`json:"ears_keyword,omitempty"\``, and `internal/ears/ears.go:classifySpecJSON` reads `ac.EARSKeyword` directly (deliberately, "does not re-derive the pattern from the AC text"), mapping it via `patternFromKeyword`, where empty/absent → `PatternUbiquitous`. This path is reachable at runtime via `sworn lint` (cmd/sworn/lint.go:93 `ears.Validate(releaseDir)`). After the strip, every non-ubiquitous AC across the five migrated releases (e.g. THIS spec's AC-05 `"ears_keyword": "When"`) reads as empty → collapses to Ubiquitous in the `sworn lint` EARS report. The strict v0.10.0 spec-v1 schema forbids `ears_keyword` (AC allowed keys are exactly `{id, text, ears_pattern, test_refs}`) and uses `ears_pattern` — but the Go read-path was never migrated to `ears_pattern`, so the strip cannot be a clean no-op. The honest fix (migrate reader+writer to `ears_pattern`; translate rather than delete) is a Go behaviour change the spec declares OUT of scope ("no Go behaviour change beyond removing the tolerance"). `go test ./...` will not necessarily catch it — ears tests use fixtures, not the live `docs/release/` specs.
   What to ask the implementer: do NOT treat P-2 as settled. Confirm whether `sworn lint` over each migrated release still classifies non-ubiquitous ACs correctly after the strip (run `sworn lint <release>` pre/post and diff the EARS distribution). The Coach must decide the disposition: (a) accept the lint EARS-classification degradation as a known, tracked precision loss, or (b) pull the `ears_pattern` reader/writer migration into scope (widens S12 beyond "no Go behaviour change"), or (c) sequence a separate slice. Tracked in sworn#95.
   Citation: —

2. [escalate] §3b/§6.P-1 — AC-02's invalid `feature` record does not exist; "SHALL be corrected" is vacuous as written (CRITICAL)
   What I observed: AC-02 states "The single record carrying the invalid 'quadrant': 'feature' … SHALL be corrected." I independently verified: `grep -rn '"quadrant": "feature"' docs/release/ --include='*.json'` returns **zero** record matches (the only textual hit is design.md's own prose). The nearest named target, render-drift `S03-tui-chrome-rework`, is `epic` (high/high), not `feature`. The AC's presupposition is false — there is no record to correct. The design proposes satisfying AC-02 by asserting absence (grep-zero + journal note) plus a defensive `chore/epic/feature`→canonical map in the committed script. The factual question ("does a `feature` record exist?") I have settled: no. The residual is a spec-fidelity judgement — a fail-closed Verifier loaded only with spec+proof would read "SHALL be corrected", find no correction, and FAIL. So this is not safely left to the Verifier backstop.
   What to ask the implementer: proceed with grep-zero evidence captured in proof.json + journal, and keep the defensive map in the script (forward-safe for ~/projects/fired re-runs). The Coach decides: accept absence-satisfaction and (recommended) `/replan-release` to strike or soften AC-02 so the Verifier does not fail-close, OR point to the branch/record where a `feature` quadrant still lives.
   Citation: —

3. [mechanical] §3e/§6.P-3 — Records-conformance Go test is an approved-inline touchpoint expansion, not a Coach call
   What I observed: AC-03/AC-06 require a validation sweep whose output is captured in proof.json; §3e correctly notes no existing CLI runs `baton.ValidateSchema` over on-disk records (verified: `ValidateSchema` exists at internal/baton/validate_schema.go:69; write paths use lenient `baton.Validate`; `sworn doctor` checks render-drift/timestamps, not schema conformance). The design proposes a records-conformance Go test globbing the five releases and asserting `ValidateSchema("spec-v1"|"board-v1", …)` — one file beyond declared `touchpoints`. This is HOW to run a spec-mandated sweep (a determinable technical choice), strengthens verification, and is Verifier-backstoppable — apply-inline, not escalate.
   What to ask the implementer: add the conformance test; note the one-file touchpoint expansion in journal.md and proof.json. It doubles as durable regression (a future un-migrated record fails CI) and Rule 1 reachability (real records through the real strict validator).
   Citation: —

4. [mechanical] §3a/§6.P-4 — Whitelist board projection (drops stray `activity`) is sound; confirm the larger board diff is expected
   What I observed: §3a reconstructs board.json as `{$schema, release, tracks: map({id, slices} + optional depends_on)}` — a whitelist, dropping `schema_version`, `release_worktree_path/branch`, `activity` (2 boards), and every track `state`/`worktree_path`/`worktree_branch`. Verified against the vendored v0.10.0 board-v1 schema: top-level allowed keys are exactly `{$schema, release, tracks}` and track keys exactly `{id, slices, depends_on}`, both `additionalProperties:false` — so `activity` and the worktree/state fields are genuinely forbidden and the whitelist is the correct, future-proof approach.
   What to ask the implementer: confirm `slices` arrays are preserved byte-for-value (board-v1 canonical-shape / string-vs-object migration is baton#54, out of scope) and that the enlarged board diff is expected, not accidental.
   Citation: [[project_board_v1_release_shape_skew]]

5. [memory-cited] §6.P-5 — In-flight-track drift is self-healing via forward-merge, not cross-worktree edits
   What I observed: P-5 and spec R-01 both state the migration lands on the integration line and reaches in-flight tracks (operational-readiness, release-hygiene) via `/implement-slice` Step-0 forward-merge self-heal; out_of_scope explicitly forbids migrating records on track branches directly (Rule 11). This aligns exactly with the replan-propagation lesson.
   What to ask the implementer: acknowledge; enumerate the touched in-flight tracks in journal.md so the next session isn't surprised by the expected "behind" reading. Propagate by forward-MERGE (`git merge --no-ff release-wt/<rel>`), never by cp-files + separate commit — the drift gate counts commit ancestry, not content.
   Citation: [[feedback_replan_propagate_by_merge_not_copy]]

6. [memory-cited] §3c/§5 — Newline-eating edit hazard on the shim removals + full-suite gate before transition
   What I observed: §3c removes two `Normalise(...)` call-site blocks (state.go ~529 and board.go ~137, both verified present with explanatory comments) and edits `effort_complexity_test.go`. This project has a recurring corruption where an edit fuses a statement onto a preceding `//` comment line, silently commenting out code. The two removals sit directly beneath multi-line `//` comment blocks — prime territory.
   What to ask the implementer: after every .go edit, `grep -nE '//.*\t+(return|[a-z]+\()'` the changed files, run `gofmt -l` + `go vet`, and run the FULL `go test -count=1 -timeout 300s ./...` before any state transition (also the standing S05 lesson: a tightened reader regresses fixtures in OTHER packages — the ~9 shim-dependent test files named in §3c must all go green). The full suite is the gate, not `go test ./internal/state/...` alone.
   Citation: [[project_newline_eating_edit_corruption]]

7. [mechanical] §6 (inter-slice) — "S12 is the LAST T7 slice" is asserted in prose only; no `depends_on` encodes it
   What I observed: the spec's v0.10.0 note relies on ordering "S11 → S15 → S12" so that S12 removes the shim + tightens Validate AFTER siblings author their records. But board.json's T7 entry carries only `depends_on: [T4,T5,T6]` — no intra-track ordering between S15 and S12, and both S15 and S12 are currently at `design_review` (S15 authored in retired `chore` vocab, per its status.json). If S12 lands before S15 is implemented, S15 then authors a `chore` record after the shim is gone and Validate is strict → S15's record fails to load. A serial implementer owning the T7 worktree resolves this by running S15 first, but nothing machine-enforces it.
   What to ask the implementer: confirm S15-baton-version-handshake is implemented/verified (its records migrated to `quick` with the rest, or authored in `quick` after the shim lands) BEFORE S12 transitions — or hold S12 as genuinely last. Optionally encode the ordering rather than leaving it in prose.
   Citation: —

8. [mechanical] Step 2b (Rule 9) — status.json carries no `design_decisions` block
   What I observed: S12's status.json has no `design_decisions` field. The design makes at least three recordable choices — the whitelist board projection (P-4), the conformance-test addition (P-3), and (pending pin 1's resolution) the disposition of the ears_keyword precision loss. This is the 5th recurrence of this exact gate-field gap this release (S04, S08, S11, S15, now S12); it is already tracked as a tooling fix in sworn#94.
   What to ask the implementer: populate `design_decisions` with the Type-2 classifications (whitelist projection, conformance test) so the Rule 9 design-fit gate passes; if the Coach accepts pin 1(a), record the accepted lint degradation as a decision with its rationale.
   Citation: [[project_driver_contract_recut]]

## Summary

Pins: 8 total — 4 [mechanical], 2 [memory-cited], 2 [escalate]
Critical pins: 1 (ears_keyword strip silently regresses `sworn lint` EARS classification), 2 (AC-02 vacuous — a fail-closed Verifier would FAIL on "SHALL be corrected")

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) `type` strip is harmless: verified no Go reader consumes AC-item `.type` (reqverify uses only `.Text`; ears uses only `EARSKeyword`) — only the `ears_keyword` strip (pin 1) regresses anything.
- (b) Chore/epic file counts: design's "38 chore files" is accurate (verified 38 files; the earlier grep line-count of 39 was noise). AC-01 is a grep-zero assertion so counts are immaterial regardless.
- (c) VERSION pin verified: `internal/adopt/baton/VERSION` reads baton-protocol v0.10.0 @ a5ab2aa — the schema S12 validates against; S11 (verified) landed it.
- (d) Five spec-v1-era releases confirmed exactly (15/6/3/2/7 spec.json); legacy markdown-era releases carry 0 spec.json and are naturally excluded — matches the Coach 2026-07-10 no-repair decision.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR strong, well-grounded migration design — every board-v1/Validate/ValidateSchema/call-site citation held exactly against live code — but two escalate pins need a Coach call before code. 8 pins + 4 flags:

1. **ears_keyword strip regresses `sworn lint` (CRITICAL — Coach).** P-2 proves `Normalise("spec-v1",…)` is a dead branch (true), but that is not the same as unread: `internal/ears/ears.go:classifySpecJSON` reads `ac.EARSKeyword` directly and maps empty→Ubiquitous, reachable via `sworn lint` (lint.go:93). Stripping `ears_keyword` (required for strict v0.10.0 — the schema uses `ears_pattern`, which the Go reader was never migrated to) silently collapses every non-ubiquitous AC to Ubiquitous across the five releases. `go test ./...` may not catch it (fixture-based). Run `sworn lint <release>` pre/post and diff. Coach disposition needed: (a) accept the degradation as tracked precision loss, or (b) pull the `ears_pattern` reader/writer migration into scope (breaks "no Go behaviour change"), or (c) separate slice. Tracked sworn#95.
2. **AC-02 is vacuous (CRITICAL — Coach).** Zero `"quadrant": "feature"` records exist in any release (verified, JSON-only). "SHALL be corrected" presupposes a record that isn't there; a fail-closed Verifier would FAIL. Capture grep-zero + journal note, keep the defensive script map, and Coach decides: accept absence-satisfaction (recommend `/replan-release` to strike/soften AC-02) or point to where `feature` lives.
3. **Conformance Go test — approved inline.** Add the records-conformance test (globs 5 releases, asserts `baton.ValidateSchema` per record); note the one-file touchpoint expansion in journal/proof. It is the AC-03/AC-06 sweep evidence + durable regression.
4. **Whitelist board projection — confirmed.** Verified v0.10.0 board-v1 forbids `activity` and the worktree/state fields; whitelist `{$schema,release,tracks:{id,slices,depends_on}}` is correct. Preserve `slices` untouched (baton#54 out of scope); expect the larger board diff.
5. **Forward-merge self-heal — acknowledged.** Reach in-flight tracks only via `/implement-slice` Step-0 forward-MERGE (never cp-files+commit — drift gate counts ancestry). Enumerate touched in-flight tracks in journal.
6. **Newline-eating hazard + full-suite gate.** After the two `Normalise` call-site removals (state.go:529, board.go:137) and the effort_complexity_test.go edit: `grep -nE '//.*\t+(return|[a-z]+\()'` the changed .go, `gofmt -l` + `go vet`, then FULL `go test -count=1 -timeout 300s ./...` (the ~9 shim-dependent fixtures must all go green) before any transition.
7. **S15 sequencing.** "S12 is last in T7" is prose-only; board.json encodes no S15→S12 ordering and both are at design_review (S15 authored in `chore`). Confirm S15 is implemented/verified (or authored in `quick`) before S12 removes the shim + tightens Validate, else S15's `chore` record fails to load.
8. **Record `design_decisions`.** status.json has none; populate the Type-2 classifications (whitelist projection, conformance test) so the Rule 9 gate passes (5th recurrence this release; sworn#94).

Flags (not pins): (a) `type` strip is harmless (no Go reader consumes `.type`); (b) "38 chore files" count is accurate; (c) VERSION pin verified v0.10.0 @ a5ab2aa; (d) five spec-v1-era releases confirmed (15/6/3/2/7).

§2 decisions: whitelist projection [[project_board_v1_release_shape_skew]], forward-merge self-heal [[feedback_replan_propagate_by_merge_not_copy]], shim-removal fixture blast radius [[project_newline_eating_edit_corruption]] acknowledged. §6 pins P-2→pin 1, P-1→pin 2, P-3→pin 3, P-4→pin 4, P-5→pin 5.

Address pins 3–8 inline during implementation. Pins 1 and 2 need a Coach decision first (ears-strip disposition; AC-02 accept-or-replan); once ruled, proceed to in_progress.

## Triage verdict

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: yes
REASON: Two escalate pins need Coach authority before code — the ears_keyword strip silently regresses `sworn lint` EARS classification and its clean fix is out of the slice's declared "no Go behaviour change" scope (sworn#95), and AC-02's "SHALL be corrected" is vacuous (zero `feature` records) so a fail-closed Verifier would FAIL absent an accept-by-absence ruling or `/replan`. Constitutional: a committed field-deleting migration reshaping 40+ record files across five releases.
-->
