# Captain review — S38-verifier-blocked-violations
Date: 2026-06-23
Design commit: db7164abf59c2d715baf286f867ca3192ec39ed6

## Pins

1. [mechanical] §3/§5 — `scripts/release-verify.sh` path is inconsistent between design sections
   What I observed: Design §3 lists `scripts/release-verify.sh` as a file to touch, but no `scripts/` directory exists in the sworn worktree (`ls scripts/` → missing). Design §5's reachability plan runs `$HOME/.claude/bin/release-verify.sh` — the baton-owned script. These are two different paths pointing to two different possible targets. If the baton-owned script (`$HOME/.claude/bin/release-verify.sh`) is the target, it is not tracked by sworn's git and will not appear in `git diff --name-only`; Verifier Gate 2 cannot validate it. If a NEW `scripts/release-verify.sh` in the sworn repo is intended, it is distinct from the baton script and §5's invocation path is wrong.
   What to ask the implementer: Before writing code, resolve: (a) update §3 to name the exact file path that will be modified, and (b) if the target is the baton-owned `$HOME/.claude/bin/release-verify.sh`, remove it from status.json `planned_files` (it won't appear in git diff) and document in the proof.md how AC2's gate is delivered outside the sworn git tree.

2. [mechanical] §3/status.json — `planned_files` will not match the actual changed files
   What I observed: status.json `planned_files` is `["internal/prompt/verifier.md", "internal/verify/verify.go"]`. But design §3 says: (a) the new Go file is `internal/verify/validate_blocked.go` (NOT `verify.go`), (b) `internal/verify/verify_test.go` is also touched, and (c) `scripts/release-verify.sh` (path TBD per Pin 1). The Verifier checks `planned_files` vs `git diff --name-only` at Gate 2. As filed: `verify.go` is in plan but won't be changed → Gate 2 FAIL. `validate_blocked.go` and `verify_test.go` will be changed but are absent from plan → Gate 2 FAIL. This will cause a guaranteed Gate 2 FAIL verdict on the first verify round.
   What to ask the implementer: Update status.json `planned_files` to exactly match design §3's file list before transitioning to in_progress. Correct: `["internal/prompt/verifier.md", "internal/verify/validate_blocked.go", "internal/verify/verify_test.go"]` plus the bash gate file once Pin 1 is resolved.

3. [mechanical] §4 NOT-doing — baton's `verifier.md` copy also needs the BLOCKED update, but design excludes it
   What I observed: Two `verifier.md` files exist: (a) `internal/prompt/verifier.md` (sworn repo — design touches this), and (b) `$HOME/.claude/baton/role-prompts/verifier.md` (baton-owned — used by the Claude Code `/verify-slice` skill). A `diff` of the two files confirms they have DIVERGED: the sworn copy has sections the baton copy does not (E2E credential guard, approved-ack.md clarification, non-gating-findings capture policy), and the baton copy has a "Status block (mandatory)" section and a `blocked_needs_planner` watcher block that the sworn copy does not. The `/verify-slice` Claude Code skill loads the baton copy. Human verifiers using the skill will not see the new BLOCKED violations requirement unless the baton copy is also updated. The design's NOT-doing reasoning ("the slash command routes to the role prompt") is accurate for the skill file, but the role prompt the skill routes to is the baton copy, not the sworn copy. The spec's parenthetical "(and `verify-slice.md` command)" anticipated this surface.
   What to ask the implementer: Confirm whether `$HOME/.claude/baton/role-prompts/verifier.md` will also receive the two-sentence BLOCKED violations requirement. If yes, add it to §3's file list. If governance (ADR-0006) requires a baton PR, note that as a Rule 2 deferral with tracking. The Go gate in `validate_blocked.go` + `release-verify.sh` will catch malformed BLOCKEDs regardless, but the prompt gap means verifiers using the Claude Code skill won't be told to populate violations — the gate is a backstop, not a substitute for the prompt instruction.

4. [mechanical] status.json — `design_decisions` field absent (4th consecutive T12 slice with this gap)
   What I observed: status.json has no `design_decisions` field. The five §2 decisions (gate location, additive-only prompt change, jq-vs-struct, forward-looking gate, test=reachability) have not been classified as Type-1 or Type-2. The trial log shows S35, S36, and S37 all had this same gap and were pinned for it. `sworn designfit` trivially passes an empty decisions array, bypassing Rule 9's Type-1 classification gate entirely. Decision 1 (gate location: release-verify.sh bash + Go unit) is potentially Type-1 (determines where enforcement lives — bash harness vs Go binary — which shapes how the gate is testable, deployable, and reachable in the run loop).
   What to ask the implementer: Add `design_decisions` to status.json with type classification for all five §2 decisions before in_progress. Specifically: assess Decision 1 (gate location) for Type-1 status — if the answer is Type-1, a human decision record is required before code.

Pins: 4 total — 4 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: Pin 2 — planned_files mismatch will cause a deterministic Gate 2 FAIL on the first verify round; this is the apply-inline fix most likely to save a verify cycle.

## Summary

4 mechanical pins, no escalates. The design's substance (prompt addition + Go gate + bash gate) is correctly scoped and directly addresses the observed failure. Pins are administrative hygiene: path consistency (Pin 1), planned_files alignment (Pin 2), dual-file coverage for the prompt update (Pin 3), and the recurring T12 design_decisions gap (Pin 4). None require a re-design — all are apply-inline during implementation.

## Smaller flags (not pins, worth one-line ack)

(a) **"at-write time" description in Decision 4**: The gate fires at validation-time (when release-verify.sh is invoked after the verifier has written status.json), not literally at-write-time. Phrasing is slightly misleading but correctness is fine.

(b) **"naming the slice" in AC2**: The spec says the check "names the slice" when it fails. The proposed `ValidateBlockedViolations()` Go function signature doesn't obviously accept a slice ID. Confirm the error return includes the slice ID, or confirm the release-verify.sh caller (which has `$SLICE_ID`) surfaces the name in its output.

(c) **verify.go already large**: `internal/verify/verify.go` is 386 lines and holds boundary-mock detection, verdict parsing, and the main `Run()` function. Adding a new `validate_blocked.go` as a separate file is the right call (design Decision 1 implicit) — don't consolidate into verify.go.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR solid concept, 4 mechanical housekeeping pins before code opens. 0 escalates.

1. **Resolve `release-verify.sh` path ambiguity.** §3 says `scripts/release-verify.sh` (doesn't exist in the repo); §5 runs `$HOME/.claude/bin/release-verify.sh` (baton-owned). Pick one: if baton-owned, update §3 to name the correct path and remove it from `planned_files` (not git-tracked); if new sworn-repo file, fix §5's invocation path. Resolve before writing any code.

2. **Update status.json `planned_files` to match design §3.** Current: `["internal/prompt/verifier.md", "internal/verify/verify.go"]`. Correct: `["internal/prompt/verifier.md", "internal/verify/validate_blocked.go", "internal/verify/verify_test.go"]` plus the bash gate file once Pin 1 is resolved. `verify.go` is not being changed — drop it. Gate 2 will FAIL if this isn't fixed before verify.

3. **Confirm whether `$HOME/.claude/baton/role-prompts/verifier.md` also gets the two-sentence BLOCKED violations addition.** The two files have diverged; the Claude Code `/verify-slice` skill loads the baton copy, not the sworn copy. If you're updating only the sworn copy, the Go gate backstops enforcement. If you're updating both, add the baton path to §3 and note any ADR-0006 governance dependency.

4. **Add `design_decisions` to status.json (recurring T12 gap — S35/S36/S37 all pinned for this).** Classify all five §2 decisions as Type-1 or Type-2. Decision 1 (gate location: bash vs Go) warrants Type-1 assessment — if Type-1, get a human decision record on the record before writing code. `sworn designfit` trivially passes an empty decisions array.

Flags (not pins): (a) "at-write time" in Decision 4 is technically "at-validation-time" — fine for correctness; (b) confirm error path from `ValidateBlockedViolations()` names the slice in its output message.

Apply pins 1–4 inline at in_progress entry, then proceed.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All four pins are apply-inline mechanical corrections (path fix, planned_files update, baton-copy confirmation, design_decisions field); none require re-checking the design before code opens — the substantive approach is sound.
-->
