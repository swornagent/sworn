# Captain review — S01-driver-contract
Date: 2026-07-03
Design commit: fbf0ff8d5fc3ad93072b3b2b9c97dce4c7ea8746

## Pins

1. [mechanical] §"Key design choices" bullet 1 — sworn#35 citation is wrong
   What I observed: design.md line ~77-78 says role-dispatch "closes the exact class of bug in sworn#35 (Claude subprocess driver advertised structured output it didn't have)". Fetched `gh api repos/swornagent/sworn/issues/35`: the actual issue is "Implement Anthropic tool-use in the Chat driver (different API shape)" — a forward-looking feature request to add tool-use support, not a bug report about a driver falsely advertising a capability it didn't have. No GitHub issue matching "advertised structured output it didn't have" was found by search (checked #22, #55, #61, #62, #15, #19, #31, #34 titles/bodies). The body of #35 references "(#— S01-captools)" — an unresolved placeholder link to a prior slice, not a filed issue — so the actual incident this alludes to may never have been captured as a numbered issue.
   What to ask the implementer: correct the citation (either find the right issue number, or rephrase without a specific issue number) before it gets restated verbatim in `docs/adr/0012-driver-contract.md` (AC-06 requires the ADR to record rationale accurately).

2. [mechanical] AssertWorktree spec — "or equivalent stat-based check" hedge is unsafe for this project's own worktree topology
   What I observed: design.md says AssertWorktree walks "path exists → is a directory → `git rev-parse --is-inside-work-tree` (or equivalent stat-based check) succeeds". I confirmed live: this track worktree's `.git` is a plain **file** (`gitdir: /home/user/projects/sworn/.git/worktrees/release-2026-06-28-driver-contract-T1-contract`), not a directory — the standard shape for every `git worktree add` checkout in this project (release-wt/, every track/ worktree). A naive "stat-based check" that looks for a `.git` directory (rather than shelling out to `git rev-parse --is-inside-work-tree`, or parsing the `gitdir:` file) will misclassify every worktree this project actually uses as "not inside a git working tree" — the opposite of AC-04's fail-closed intent, and directly relevant to CLAUDE.md Rule 11 (Process-Global Mutation Guard), which this helper exists to satisfy.
   What to ask the implementer: commit to the `git rev-parse --is-inside-work-tree` implementation (or a stdlib check that correctly handles the `.git`-as-file linked-worktree shape) — not a naive directory-presence stat check — and add a `TestAssertWorktree` case exercising a linked worktree (`.git` file, not dir) as a success case, not just a plain checkout.

3. [mechanical] AC-01 field-type reading — confirm `Role` as a named type satisfies the spec text
   What I observed: spec.json AC-01 groups "fields Role, ModelID, SystemPrompt, Payload (strings)" — read literally this could mean `DispatchInput.Role` is Go `string`. design.md's Go snippet instead types it as the named type `Role` (`type Role string`), consistent with AC-02's "declared set" language and `RoleSet map[Role]bool`. Design.md's own risk note explicitly asks the reviewer to check AC-01 "line by line before acknowledging" — this is that check.
   What to ask the implementer: confirm the named-type reading is intentional (near-certain given AC-02 and RoleSet, but AC-01 is flagged load-bearing by the design itself, so make the confirmation explicit rather than assumed).

## Summary

Pins: 5 total — 3 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins (if any): none — all 3 mechanical pins are apply-inline corrections (a citation fix, an implementation-approach confirmation, and a type-reading confirmation), none require re-checking the design itself.

## Smaller flags (not pins, worth one-line acknowledgement)

- design.md uses non-standard section headers (Approach / Key design choices / Files touched / Design-level risks / AC traceability) instead of the template's §1–§6 numbering. No NOT-doing or explicit reachability/open-questions sections, but the substance is covered elsewhere (reachability = `go build ./...` + `go test ./internal/driver/...`, stated in AC traceability). Not blocking — a template-conformance nit only.
- ADR-0011 (structured-output keystone) is referenced 30+ times across `internal/` (verify.go, model/*, baton/*, run/*, orchestrator/*, state/*, verdict/*) as a ratified, load-bearing decision with quoted subsections (§3.3 g, §3.7, §2 finding 1) — but `docs/adr/0011-*.md` does not exist on any branch. Not S01's defect (predates this slice), but S01 is about to land `docs/adr/0012-driver-contract.md` immediately after this gap. Filed as [swornagent/sworn#79](https://github.com/swornagent/sworn/issues/79) per capture discipline; not a pin because it does not block this slice's own delivery.
- `RoleSet` is `map[Role]bool`, a mutable reference type returned by `Roles()`. Normal Go idiom, not spec-required to guard against post-resolution mutation. No action needed, just noted.

## Suggested acknowledgement reply

TL;DR Clean, well-grounded design that matches the recorded Type-1 decision exactly — the two ambiguities it invites the reviewer to check (AC-01 field types, AssertWorktree's implementation approach) both resolve in the design's favor once checked against live repo state, plus one citation to fix. 3 pins + 3 flags:

1. **Fix the sworn#35 citation.** The bug you're citing as motivation for role-dispatch isn't issue #35 (that's "Implement Anthropic tool-use" — a feature request, not a false-advertising bug). Either find the right issue number or drop the specific citation before it lands in ADR-0012.
2. **Commit to `git rev-parse --is-inside-work-tree` for AssertWorktree, not a stat-based directory check.** Every worktree in this project (including the one you're implementing in right now) has `.git` as a file, not a directory — a naive stat check would fail-open... no, fail *closed* on valid worktrees, which is the wrong failure. Add a linked-worktree test case to `TestAssertWorktree`.
3. **Confirm `DispatchInput.Role` as the named type `Role`, not raw `string`.** AC-01's "(strings)" parenthetical is ambiguous; your named-type reading matches AC-02/RoleSet and is almost certainly right — just confirm explicitly since AC-01 is flagged load-bearing.

Flags (not pins): (a) design.md's section headers don't match the §1–§6 template numbering, but the content is all there; (b) ADR-0011 is cited pervasively in the codebase but the file doesn't exist anywhere — tracked as swornagent/sworn#79, not this slice's problem to fix, just worth knowing before you number ADR-0012; (c) `RoleSet` is a mutable map, no action needed.

§2 decisions: role-dispatch shape (decision 1) and engine-owns-verdict-validation (decision 3) both [[project_driver_contract_recut]] / [[project_keystone_structured_outputs]] memory-cited and confirmed accurate against live code (`internal/verify/verify.go:200`, `internal/model/structured.go`). Wire-types-internal (decision 2) and no-registration-this-slice (decision 4) acknowledged clean, no memory conflicts.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 3 pins are apply-inline corrections (one citation fix, two confirm-and-proceed checks) resolvable during implementation; the Type-1 decision is already recorded with options+rationale and the design faithfully implements it — no design-changing or Coach-authority pins.
-->
