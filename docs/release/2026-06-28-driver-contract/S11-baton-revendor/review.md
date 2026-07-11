# Captain review — S11-baton-revendor
Date: 2026-07-11
Design commit: 6fe63fae6ed65010514c699eec64136a9970639d

## Pins

1. [mechanical] §46 (D1–D5) — status.json carries no `design_decisions`; the Rule 9 design-fit gate fails closed on the two Type-1 choices.
   What I observed: design.md declares D1 (Type-1), D2 (Type-1), D3 (escalate), D4 (Type-2), D5 (Type-2), but S11's `status.json` has no `design_decisions` field at all. Verified live: siblings S01-driver-contract, S05-driver-registry, S09-model-catalog each carry `design_decisions` in their `status.json`, so the field is an established release convention, not an optional one. Rule 9's design-fit gate fails closed on any Type-1 choice with no recorded human decision.
   What to ask the implementer: Populate `status.json.design_decisions` with D1–D5, each carrying its classification and, for the Type-1 entries (D1, D2), the recorded Coach decision. D1's recorded decision is the tolerance mechanism the Coach picks in pin 2; D2's is "reuse worker.go sibling logic" (pin 4). Do this before `in_progress`.

2. [escalate] §48 (D1) — transitional-tolerance mechanism is a genuine Type-1 with two viable options and no single right answer; the design itself requests a human decision.
   What I observed: S11 vendors the strict v0.9.0 schemas (verified against tag v0.9.0: spec-v1 and board-v1 both `additionalProperties:false`, neither defines `schema_version`, board-v1 `tracks[]` items = `{id, slices, depends_on}` only) but S11 does NOT migrate live records — S12 owns that. The moment S11 lands, un-migrated records (this release's own `board.json` carries per-track `worktree_path`/`worktree_branch`/`state` + top-level `schema_version`; `spec.json`/`status.json` carry `schema_version`; the quadrant fields read `epic`) would be rejected by strict `additionalProperties:false` validation, and the quadrant checksum would derive `beast` where the record says `epic`. Because S11+S12 merge as one track unit the integration branch never sees the intermediate state, but in-track gates (`sworn verify`, `sworn board`, designfit, fixtures) operating on the still-live records will. Design D1 proposes two mechanisms — (a) a transitionally-tolerant validation path that does not enforce `additionalProperties:false`/`schema_version`-absence against un-migrated records, or (b) normalise-before-validate — and explicitly classifies the choice Type-1, "architecturally significant and load-bearing for the in-flight release," requesting a human decision.
   What to ask the implementer: Do not proceed on this decision without the Coach. Option (a) keeps a tolerant validation path (removed by S12); option (b) strips the retired fields before validating. Both must be reverted/tightened in S12. The Coach picks the mechanism; record it as D1's Type-1 `design_decision` (pin 1).

3. [escalate] §50 (D3) / spec AC-08 — AC-08 targets vendored command specs that sworn does not vendor; the AC as written cannot be satisfied literally.
   What I observed: AC-08 and the ninth in_scope item require "the vendored implement-slice.md and merge-track.md command specs (internal/adopt/baton/rules/)" to no longer instruct board.json writes. Verified live: sworn's `internal/adopt/baton/rules/` contains ONLY the eleven numbered rule docs (`01-…`..`11-…`); `internal/baton/source.go` `batonFileMappings` maps no `commands/`; no `implement-slice.md` or `merge-track.md` exists anywhere under sworn's vendored roots. Upstream v0.9.0 does carry `commands/implement-slice.md` + `commands/merge-track.md`, but sworn has never vendored `commands/`. So AC-08's named artefact does not exist in-repo. Design D3 recommends treating AC-08 as satisfied-by-engine (AC-06/AC-07 ensure no sworn writer stamps track worktree/state to board.json) plus a Rule 2 note that the command-spec prose lives in the private `~/.claude` harness, and requests the Captain's ruling.
   What to ask the implementer: This is a spec-coherence deviation, not a code choice — surface to the Coach. The determinable fact (sworn vendors no `commands/`) is settled. The Coach rules between: (a) accept AC-08 satisfied-by-engine, with a Rule 2 note (filed as an issue) that command-spec write-isolation lives in the private harness, out of this repo; or (b) `/replan-release` to rewrite AC-08 to target the engine, or to expand scope to add a `commands/` mapping. The Captain does not originate the spec change.

4. [memory-cited] §49 (D2) — track-path derivation must reuse worker.go's sibling-of-release-worktree logic, not the naive `$HOME` formula.
   What I observed: Design D2 derives the track worktree path as a sibling of the release worktree, reusing `internal/scheduler/worker.go` `defaultTrackWorktreePath`, explicitly NOT track-mode.md's `$HOME/projects/<repo>-worktrees/…` formula. Verified live at worker.go:698–711: when `releaseWorktreePath != ""` it returns `filepath.Join(filepath.Dir(releaseWorktreePath), "release-"+releaseName+"-"+trackID)` — the sibling path — and the `$HOME/projects/<projectDir>-worktrees` branch is only the fallback reached when no release worktree path is known; the function comment attributes that fallback to the eval-finding-3 cross-repo collision. `internal/board/board.go` already holds `ReleaseWorktreePath`, so the new helper has the input it needs. The design gets this right.
   What to ask the implementer: Confirm the new `internal/board` path helper takes the release worktree path and returns `filepath.Dir(releaseWTPath)/release-<release>-<track>` — reuse worker.go's logic, do not re-derive the convention. Acknowledging confirms the citation.
   Citation: [[project_parallel_cold_start_broken]] (eval finding 3, docs/captures/2026-06-28-sworn-eval-findings.md)

5. [mechanical] §36 / AC-07 — deleting worker.go's `defaultTrackWorktreePath` fallback must sweep callers first; worker.go changed under S14 since base.
   What I observed: AC-07 removes worker.go's `defaultTrackWorktreePath` fallback once `internal/board` always returns a populated path. worker.go was touched by S14-blocked-terminal (6c5866a) since the release base, and `defaultTrackWorktreePath` has a live caller at worker.go:212. The design plans "move/share the logic first, then delete — no dead code."
   What to ask the implementer: Before deleting, `grep -n defaultTrackWorktreePath internal/scheduler/` to enumerate callers (currently the call at :212), move the sibling logic into the shared `internal/board` helper, repoint every caller, then delete — leaving no orphaned caller and no dead code. Re-run `go test -count=1 ./internal/scheduler/...` after, and the full `go test -count=1 -timeout 300s ./...` before any transition (project hazard).

## Summary

Pins: 5 total — 2 [mechanical], 1 [memory-cited], 2 [escalate]
Critical pins (if any): 2 (D1 tolerance mechanism — unresolved, in-track gates reject the release's own un-migrated records and the slice wedges verification or ships broken). Pin 1 is gate-blocking (Rule 9 design-fit fails closed) but mechanically resolvable once pin 2 is decided.

## Smaller flags (not pins, worth one-line acknowledgement)

(a) Line-number drift in the artefacts (not design.md): spec `rationale` cites `state.go:410,426` and R-05 cites `worker.go:600-621`; live is `Quadrant()` at state.go:428 (returns at 431–437) and `defaultTrackWorktreePath` at worker.go:698. design.md itself uses the correct :698 — cosmetic, re-anchor if those quotes are copied forward.
(b) schema_version emit-site: `internal/spec/spec.go` does not appear to emit `schema_version` (the only occurrence under internal/spec is the spec_test.go fixture), yet live `spec.json` records carry `schema_version: 1` and `internal/implement/spec_record.go` hardcodes `SchemaVersion: 1`. Before claiming "the writer stops emitting schema_version," locate the actual emit site (likely the planner/marshal path), and make the reader tolerate absence rather than hardcoding 1.
(c) The transitional tolerance (quadrant `chore≡quick`/`epic≡beast`, `schema_version` with-or-without, board worktree/state fields) is a Rule 2 deferral that MUST be removed/tightened by S12 — confirm it is tracked (sworn#90 / the S12 spec) so tolerance cannot silently ship past S12.
(d) Project hazards apply to the AC-05 sweep: `scripts/release-verify.sh` false-FAILs "spec.md missing" on spec-v1 slices and sworn#87's boundary_mock scanner false-positives on prose — declare, don't contort; and grep changed `.go` files for code fused onto `//` lines (newline-eating corruption) + `gofmt -l`/`go vet` on changed packages.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Strong, unusually well-grounded design — citations to worker.go, the v0.9.0 schemas, and the two sync pipelines all held on live-repo check. Two decisions need the Coach before code; three pins apply inline. 5 pins + 4 flags:

1. **Record design_decisions (Rule 9 gate).** Add a `design_decisions` array to `status.json` capturing D1–D5 with their classifications; siblings S01/S05/S09 all carry this field and the design-fit gate fails closed without it. Type-1 entries (D1, D2) must carry the recorded decision — D1 from the Coach ruling below, D2 = "reuse worker.go sibling logic".

2. **Transitional-tolerance mechanism (Coach decision required).** COACH DECISION → [pick one]: (a) transitionally-tolerant validation path — do not enforce `additionalProperties:false`/`schema_version`-absence against un-migrated records until S12; or (b) normalise-before-validate — strip retired fields before validation. Same transitional pattern as the quadrant `epic≡beast` equivalence; whichever is chosen, S12 removes it. Record the choice as D1's Type-1 design_decision, then implement it.

3. **AC-08 command specs (Coach ruling required).** sworn vendors no `commands/` — `internal/adopt/baton/rules/` holds only the numbered rule docs, so AC-08's named `implement-slice.md`/`merge-track.md` do not exist in-repo. COACH RULING → [pick one]: (a) accept AC-08 satisfied-by-engine (AC-06/AC-07 ensure no sworn writer stamps track worktree/state to board.json) + file a Rule 2 tracking issue that command-spec write-isolation lives in the private `~/.claude` harness; or (b) `/replan-release` to rewrite AC-08 against the engine. Do not expand scope to vendor `commands/` without ruling (b).

4. **Worktree-path helper reuses worker.go.** Confirmed correct: the new `internal/board` path helper takes the release worktree path and returns `filepath.Dir(releaseWTPath)/release-<release>-<track>` (worker.go:698 sibling logic) — NOT the `$HOME/projects/<repo>-worktrees` formula that caused eval finding 3. board.go already holds `ReleaseWorktreePath`. Acknowledging confirms [[project_parallel_cold_start_broken]].

5. **Delete worker.go fallback safely.** Sweep `defaultTrackWorktreePath` callers (currently worker.go:212) first, move the logic into the shared `internal/board` helper, repoint, then delete — no orphaned caller, no dead code. worker.go changed under S14 since base, so re-grep. Full `go test -count=1 -timeout 300s ./...` before any transition.

Flags (not pins): (a) stale line-numbers in spec.rationale/R-05 (`state.go:410,426`, `worker.go:600-621`) — live is 428 and 698; design.md is correct, re-anchor if copied; (b) find the real `schema_version` emit site before claiming the writer stops emitting it, and make the reader tolerate absence rather than hardcoding `SchemaVersion:1`; (c) confirm the transitional tolerance is tracked to S12 (sworn#90) so it cannot silently ship; (d) release-verify.sh spec.md-missing false-FAIL + sworn#87 boundary_mock prose false-positive — declare, don't contort; grep for `//`-fused code + gofmt/vet on the many `.go` edits.

§2 decisions: D2 [memory-cited: project_parallel_cold_start_broken] and D4/D5 (Type-2, proceed) acknowledged; D1 and D3 held for Coach. §6: no open questions beyond the design's own D1/D3 escalations, addressed above.

Address pins 1, 4, 5 and flags inline during implementation; apply the Coach's decisions on pins 2 and 3, then proceed to in_progress.

## CAPTAIN-VERDICT
<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: D1 transitional-tolerance mechanism is a genuine Type-1 choice (two viable options, no single right answer, load-bearing for the in-flight release) the model may not record itself; AC-08 (D3) is a spec-coherence deviation — its named vendored command specs do not exist in sworn — needing a Coach satisfied-by-engine-vs-replan ruling.
-->
