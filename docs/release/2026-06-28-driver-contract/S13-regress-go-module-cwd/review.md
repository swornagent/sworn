# Captain review — S13-regress-go-module-cwd
Date: 2026-07-10
Design commit: 1a6af7433d3cbb72889707d9a560134dae18a5fa

## Pins

1. [mechanical] §Approach.2 (skip branch) — skip-reason string is factually wrong for a depth-≥2 module.
   What I observed: Discovery is scoped to root + first-level subdirs (D2, matching spec in_scope "repo-root go.mod, else a single-level subdir"), but the planned skip reason is "no go.mod found under worktree". For a repo whose only go.mod sits at depth ≥ 2 (e.g. `<worktree>/src/svc/go.mod`), that message is false — a go.mod does exist under the worktree; it is just outside the scan bound. AC-03 itself is unaffected (its WHERE clause covers the no-go.mod-anywhere case, which the design handles correctly).
   What to ask the implementer: make the reason state the actual bound, e.g. "no go.mod at worktree root or in a first-level subdirectory", so a deep-module repo's skip is diagnosable instead of misleading. Apply inline.

2. [mechanical] §Design decisions — D1/D2 are not recorded in status.json `design_decisions`.
   What I observed: design.md classifies D1 (multi-module → skip-with-reason) and D2 (discovery depth = root + one level) as Type-2 noted defaults — the classification is correct (narrow, reversible, spec-aligned) — but S13's status.json carries no `design_decisions` field. Verified siblings S01–S03 all record theirs (choice / stake_class / options), and the design-fit gate reads status.json, not design.md.
   What to ask the implementer: add D1 and D2 to status.json `design_decisions` (stake_class "Type-2", noted default, one-line rationale each) when transitioning to in_progress. Apply inline.

3. [memory-cited] §Verification plan — the planned fixture edit is exactly the known cross-package-regression failure mode; the full-suite sweep commitment is load-bearing, and I verified the one real cross-package consumer tolerates the behaviour flip.
   What I observed: adding a root go.mod to existing internal/gate fixtures (the design's "required consequential edit") is the same class of change that broke board.json string fixtures in internal/board + cmd/sworn during S05 — a tightened reader/contract regressing fixtures in OTHER packages. The design already commits to the full `go test -timeout 120s ./...` sweep plus the newline-corruption grep and `gofmt -l`/`go vet`. I additionally verified the concrete cross-package exposure: `cmd/sworn/regress_test.go` (TestRegressDefaultResolution_BoardJSON / _LegacyIndexMDFallback) runs the REAL RunRegress against no-go.mod temp dirs, so its Go suite flips FAIL → Skipped after this slice — those tests assert only `exit != 2` ("exit 0 or 1"), so they tolerate the flip.
   What to ask the implementer: acknowledge that the full-suite sweep is mandatory before `implemented` (not optional polish), precisely because of this fixture-edit shape.
   Citation: [[project_newline_eating_edit_corruption]], [[feedback_releaseverify_specmd_false_fail]]

Pins: 3 total — 2 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: none — no pin would ship the slice broken if unaddressed; pin 1 affects diagnosability of a skip message, pins 2–3 are process/verification discipline.

## Summary

Pins: 3 total — 2 [mechanical], 1 [memory-cited], 0 [escalate]. No critical pins. Design verified against live code: regress.go:120-121 `runner.Run(worktree, "go", "test", "./...")` matches the cited defect; mockRunner keying (`dir + "/" + name + " " + args`, unknown-key default `("", -1, nil)`) makes the "root dir NOT used" assertion sound; existing fixtures genuinely lack go.mod so the consequential edit is real and correctly identified; `os`/`path/filepath` already imported (no-new-deps claim verified); D1 is consistent with spec R-01 mitigation and out_of_scope[1]; touchpoint-disjoint claim verified (no sibling spec lists internal/gate/regress*.go; zero commits on those files since release/v0.1.0).

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) cmd/sworn/regress_test.go's "resolves fast" comment on its temp-dir worktrees becomes actually true after this slice (Go suite skips instead of running `go test` in an empty dir) — no assertion change needed, verified.
- (b) Rule 11 posture is correct by construction: cmd.Dir stays an explicit argument through testRunner.Run; no os.Chdir or ambient-cwd mutation is introduced by this design.
- (c) Discovery reading the real filesystem (os.ReadDir) rather than the testRunner is the right seam — fixtures are real t.TempDir() layouts, so hermeticity holds without widening the runner interface.
- (d) TestRunRegress_Mixed and TestRunRegress_NoPackageJSON are covered by the design's "etc." — both also need the root go.mod fixture edit or their tallies shift.
- (e) The design-review LLM check (`sworn llm-check --type design-review`) could not run in this session: no model configured (`$SWORN_MODEL` unset, no --model key available). Same environment condition recorded for S03 (aa44a31, "first-pass gate unavailable (no model key)"). The pin-driven review above is the sole design-gate evidence; the Coach may re-run the LLM check in a keyed environment before acknowledging if desired.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Solid, spec-faithful design — approach, resolution order, and test mechanics all verified against live code. 3 pins + 4 flags:

1. **Honest skip reason for the scan bound.** The skip reason "no go.mod found under worktree" is false for a repo whose only go.mod sits at depth ≥ 2 (exists under the worktree, outside your root+level-1 scan). Word it to state the bound, e.g. "no go.mod at worktree root or in a first-level subdirectory".
2. **Record D1/D2 in status.json.** Add both decisions to status.json `design_decisions` (stake_class "Type-2", noted default, one-line rationale each) when you transition to in_progress — the design-fit gate reads status.json, not design.md; match the shape S01–S03 used.
3. **Full-suite sweep is mandatory, not polish.** Your fixture edit (root go.mod into existing gate fixtures) is the exact cross-package-regression shape that bit S05. Run the full `go test -timeout 120s ./...` before claiming implemented. FYI: cmd/sworn/regress_test.go's resolution tests run the real RunRegress against no-go.mod temp dirs and will flip Go FAIL → Skipped after your change — verified they assert only `exit != 2`, so they tolerate it; no edit needed there.

Flags (not pins): (a) the "resolves fast" comment in cmd/sworn/regress_test.go becomes true post-change, no action; (b) Rule 11 posture correct — cmd.Dir explicit, no os.Chdir; (c) real-filesystem discovery (not via testRunner) is the right hermetic seam; (d) TestRunRegress_Mixed and TestRunRegress_NoPackageJSON also need the root go.mod fixture edit — make sure "etc." includes them.

§2 decisions D1 [clean, spec-consistent with R-01 mitigation + out_of_scope], D2 [clean, spec in_scope wording], skip-list [clean] acknowledged. §6 empty — no open questions — acknowledged.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All pins are apply-inline corrections (a skip-message wording fix, a status.json recording step, a verification-discipline acknowledgement); the approach matches the spec mitigation exactly and every cited symbol verified against live code.
-->
