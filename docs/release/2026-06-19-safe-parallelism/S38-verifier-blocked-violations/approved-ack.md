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
