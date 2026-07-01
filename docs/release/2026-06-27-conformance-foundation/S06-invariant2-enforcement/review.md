# Captain review — S06-invariant2-enforcement
Date: 2026-06-28
Design commit: b0d635de8a49fffd41bb70af8454cddb74e8c7a1

## Pins

1. [mechanical] §"Files to touch"/§2.1 — AC-5 mock-oracle test has no injection seam (CRITICAL)
   What I observed: The design reads planned_files from committed status.json via `git show` on
   the release-wt ref (Choice 1). But spec AC-5 + Required tests demand a unit test "with a mock
   oracle returning overlapping planned_files." parallel.go's own comment (lines 170–173) states
   unit tests run OUTSIDE a real repo (opts.Router stays nil there). There is no ParallelOptions
   field through which a test can inject planned_files: board.SliceState does NOT carry
   planned_files (internal/board/oracle.go:40) and board.OracleReader exposes only
   ReadSliceStatus/ReadBoard — neither yields planned_files. Every existing test injects via a
   function/interface seam (RunSliceFn, Router, MergeTrackFn). The design specifies none.
   What to ask the implementer: Add an injection seam following the established idiom in this exact
   file — e.g. a `PlannedFilesFn func(ctx, trackID string) ([]string, error)` on ParallelOptions
   defaulting to the git-show reader — so TestInvariant2_* can inject overlapping/disjoint sets
   without real git. Without it AC-5 ("mock oracle") is not deliverable as written.

2. [mechanical] §2.2 — DOCUMENTED SHARED matrix is in the index.md body, not the frontmatter
   What I observed: Choice 2's rationale says "extending the frontmatter parser to extract the
   touchpoint matrix is incremental." But the index.md frontmatter ends at line 49 (second `---`);
   the DOCUMENTED SHARED touchpoint matrix is at lines 76–100, in the markdown BODY.
   `extractFrontmatter` (parallel.go:291) returns only lines 1–49 and discards the body — so the
   matrix is not in `fm` at all. The full file IS in scope as `string(indexData)` (parallel.go:122).
   What to ask the implementer: Parse the matrix off the raw `indexData` body, NOT the extracted
   `fm`. Don't extend extractFrontmatter — it returns the wrong slice of the file.

3. [mechanical] §status.json — Rule 9 design-fit gate fails closed: no `design_decisions` field
   What I observed: design.md §"Key design choices" enumerates 5 decisions, but S06's status.json
   has no `design_decisions` field at all. The Rule 9 design-fit gate reads that field; absent, it
   cannot classify and fails closed.
   What to ask the implementer: Populate `design_decisions` in status.json, classifying each. None
   of the five touches auth/payments/PII/migration/irreversible — Type-2 is defensible for all —
   but the field must EXIST and each architecturally-relevant choice (the block/retry control-flow
   change to RunParallel) must carry its classification + rationale.

4. [mechanical] §2.3 — Pin the exact error-string the AC-5 test asserts
   What I observed: spec gives two forms of the report: the in-scope bullet (line 21) is
   "INVARIANT-2: tracks <T_a> and <T_b> both write <file> — blocked T_b until T_a merges"; AC-1
   (line 36) is the prefix only "...both write <file>". The design uses the longer form.
   What to ask the implementer: Confirm the test asserts a substring BOTH spec forms share (the
   prefix through "both write <file>"), so a later wording tweak to the suffix can't silently
   diverge message from test.

## Summary

Pins: 4 total — 4 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: 1 (AC-5 mock-oracle injection seam — slice cannot deliver AC-5 as written without it)

No [memory-cited] pins: project memory is empty — no entries exist to cite.

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) Choice 1's "the `repo` variable constructed at line 175" is not reusable: `repo` is scoped
  inside the `if opts.Router == nil` block (parallel.go:174–184), unreachable by a helper
  elsewhere. The helper should construct its own `git.New(absRoot)` (the file already imports
  `git`). Pairs with pin 1.
- (b) The DOCUMENTED SHARED rows carry extra text (e.g. `` `internal/model/oai.go` + drivers
  (DOCUMENTED SHARED) ``); a "first backtick path" parser captures one path/row — fine for
  parallel.go's scope (no driver file is in T1's planned_files) but it won't capture the
  additional driver files. Note the limitation rather than silently miss them.
- (c) No §6 open-questions section; the design folded open items into self-resolved "Design-level
  risks/pins" (fail-open per spec, parser tolerance, first-track-wins). No Coach decision needed.

## Verified during review (technical facts, not pins)

- Touchpoint collisions: only S04 (verified, merged) and S06 share `internal/run/parallel.go`.
  S27-parallel-dispatch-fix (state `implemented`, same track) touches slice.go/oai.go/test files,
  NOT parallel.go — no active collision. No depends_on serialisation pin needed.
- Cross-release ancestry on parallel.go (release/v0.1.0..HEAD): S04 phase-barrier+auto-merge,
  telemetry drift merge, eventDB wiring. The design's reliance on the S04 phase barrier and
  finishTrack→MergeTrackFn auto-merge is accounted for by these merged commits.
- Block/retry mechanic (Choices 3/4) is internally sound: invariant-2 is a within-phase concern
  (same-phase tracks are the independent, concurrently-fanned-out ones); the follow-up-phase
  re-launch after `wg.Wait()` correctly defers a blocked track until the conflicting track has
  finished + auto-merged. The running-union must be scoped per-phase for this to hold — confirm
  in implementation.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads
     everything between this heading and the next ## heading (or EOF). Verbatim-pasteable into the
     Implementer session — no surrounding prose. -->

TL;DR Sound design — approach (read committed planned_files, block within-phase, retry via the
S04 phase barrier) is correct and AC-traced. 4 mechanical pins + 3 flags, all apply-inline:

1. **AC-5 test seam (critical).** Your git-show reader has no injection point for the "mock
   oracle" the spec demands, and parallel.go unit tests run outside a real repo. board.SliceState
   doesn't carry planned_files and OracleReader doesn't expose it. Add a seam following this file's
   own idiom — e.g. `PlannedFilesFn func(ctx, trackID) ([]string, error)` on ParallelOptions,
   defaulting to the git-show reader — so TestInvariant2_* inject overlap/disjoint without real git.
2. **index.md parsing target.** The DOCUMENTED SHARED matrix is in the markdown BODY (lines
   76–100), not the frontmatter (ends line 49). Parse `string(indexData)` directly; don't extend
   extractFrontmatter — it discards the body.
3. **status.json design_decisions.** Add the `design_decisions` field (currently absent) and
   classify all five choices Type-2 with rationale, so the Rule 9 design-fit gate has something to
   check.
4. **Error-string assertion.** Spec gives two forms of the message (in-scope bullet vs AC-1);
   assert the shared prefix through "both write <file>" so message and test can't drift.

Flags (not pins): (a) the `repo` var at parallel.go:175 is scoped inside `if opts.Router == nil` —
not reusable by a helper; construct `git.New(absRoot)` in the helper; (b) DOCUMENTED SHARED rows
carry extra text ("oai.go + drivers") so a first-backtick parser catches one path/row — fine for
parallel.go's scope, note the limitation; (c) no §6 — open items folded into self-resolved design
risks, none needs a Coach call.

§2 decisions 1–5 acknowledged (no memory to cite — project memory is empty). No §6 questions —
acknowledged.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Sound, AC-traced design; all 4 pins are apply-inline mechanical corrections (test seam, parse target, design_decisions field, message-string assertion) the implementer fixes in one pass with the Verifier as backstop. No spec deviation, no judgement call, no constitutional domain.
-->
