# Captain review — S01-vendor-boundary-readiness
Date: 2026-07-16
Design commit: 202338b0d6c6b6fffd6faee05db2b77beb83b5b4

## Pins

1. [memory-cited] §2.1 — The staged bootstrap remains under recorded Coach authority
   What I observed: The revised design preserves the ratified Type-1 choice to build under exact v0.15 planning records and defer automated authority until S13 revalidates the engine. That matches the project memory that Rule 9 obtains cross-cutting authority from a recorded human decision rather than allowing the loop to invent it.
   What to ask the implementer: Preserve the recorded staged-bootstrap boundary during implementation: S01 may build and prove vendor machinery, but it must not claim current protocol authority or bypass S13 revalidation.
   Citation (if [memory-cited]): [[feedback_rules_capture_not_omniscience]]

Pins: 1 total — 0 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): none

## Summary

Pins: 1 total — 0 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins (if any): none

## Smaller flags (not pins, worth one-line acknowledgement)

- The revised design owns all twelve ratified touchpoints and gives every file an AC-linked responsibility.
- The public exit map is exhaustive: exit 0 only for byte-identical check or successful write, exit 1 only for valid non-empty check drift, and exit 2 for every invalid, operational, preflight, apply, rollback, or recovery outcome, including completed recovery-only guidance.
- VERSION bytes are constructed purely from one captured invocation instant before mutation and enter the same byte-sorted snapshot/apply/rollback/verification/recovery plan as mapped destinations; there is no standalone post-vendor pin write.
- Recovery is non-self-referential and confined beneath the physically resolved current-worktree Git administrative directory, with owner-only modes, a manifest digest independent of its sentinel, complete-set validation, and fail-closed tamper, traversal, symlink, mode-drift, missing, duplicate, and foreign-material rejection before any destination touch.
- The exact upstream v0.15.1 `board-v1` bytes remain unchanged; only the decoded unsupported ECMA-262 expression receives the explicit equivalent predicate and the required accepted/rejected path matrix.
- S02 remains the sole owner of executing the v0.15.1 content/pin replacement and installing Codex/Claude mirrors. S01 changes construction and transaction machinery only.
- S05 later shares `cmd/sworn/baton.go`, `internal/baton/version.go`, and `internal/baton/version_test.go`, but it is still planned in the same serial track, so no dependency change is required; it must preserve S01's hunks.
- `sworn designfit 2026-07-15-baton-v0.16-conformance` passes all 18 slices.
- BLOCKED: pre-cutover optional LLM check not executed. The v0.15 spelling returned `flag provided but not defined: -check`; the reachable legacy adapter returned `sworn llm-check: model setup: model: no API key for provider "anthropic" — set ANTHROPIC_API_KEY, or add it to /home/brad/.config/sworn/credentials.json (run 'sworn init')`.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR The revised design is complete and mechanically sound. 1 confirmation pin + 8 flags:

1. **Preserve staged bootstrap authority.** Implement S01's vendor machinery and proof without claiming current protocol authority or bypassing the Coach-ratified S13 revalidation boundary.

Flags (not pins): (a) all twelve ratified touchpoints are owned; (b) the public 0/1/2 exit map is exhaustive; (c) VERSION is purely constructed from one captured instant and participates in the one transaction; (d) recovery is non-self-referential, Git-admin-confined, owner-only, complete-set validated, and fail-closed; (e) exact normative schema bytes are preserved; (f) S02 retains content/pin/install execution; (g) S05's later shared files are serial; (h) design-fit passes. The optional pre-cutover LLM check was not executed because the v0.15 adapter spelling is unsupported and the legacy adapter has no configured Anthropic API key; no model PASS is claimed.

§2 decisions, including the recorded Type-1 staged bootstrap and all five clean Type-2/boundary choices, acknowledged. §6 review items 1–5 are mechanically satisfied by the revised design.

Address pin 1 inline during implementation, then proceed to in_progress.

## Triage verdict

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: The revised design matches the ratified transaction, recovery, schema-byte, public-exit, and S02 boundaries; its only pin is an apply-inline acknowledgement of already-recorded bootstrap authority.
-->
