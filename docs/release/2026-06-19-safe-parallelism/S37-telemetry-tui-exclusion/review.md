# Captain review — S37-telemetry-tui-exclusion
Date: 2026-06-23
Design commit: 4aa55f0e6077b7519b4aea15680224e22806808b

## Pins

1. [mechanical] §2b.design_decisions — design_decisions field absent from status.json; five §2 decisions not populated in the designfit-gated field.
   What I observed: status.json has no design_decisions array. The design has five §2 decisions (all Type-2), but `sworn designfit` reads from status.json — an absent field means the gate runs against empty data. Trial log precedent: S35 and S36 (same track) were both pinned identically for this.
   What to ask the implementer: populate the design_decisions array in status.json with the five §2 decisions before writing code. All five are Type-2 with clear alternatives and rationale already stated in design.md §2. Copy the structure from S35's or S36's status.json as the template.

## Summary
Pins: 1 total — 1 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none — this pin does not affect implementation soundness; the design is correct and the fix is paperwork-only.

## Smaller flags (not pins, worth one-line ack)

(a) S26-telemetry (verified, T9-merged) owns both planned files in its actual_files list. This is already resolved — S26's changes are live in the worktree and the design correctly builds on them. No action needed; just confirm telemetry.go has the S26-verified shape before adding the new check.

(b) The reachability artefact is `go test ./internal/telemetry/... -v` output. This is spec-prescribed ("Verifiable by: a unit test") and appropriate for a package-internal exclusion — there is no UI surface to screenshot. Accepted as the correct artefact form for this slice.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR design is tight and correct — 1 pin + 2 flags:

1. **design_decisions missing from status.json.** Before writing any code, populate the design_decisions array in status.json with the five §2 decisions. All five are Type-2 — copy the structure from S35's or S36's status.json. The designfit gate reads this field; absent = trivially-passing but audit-blind.

Flags (not pins): (a) S26-telemetry is the prior owner of both planned files — confirmed S26 changes are live in this worktree and your changes extend them additively; (b) reachability artefact = `go test ./internal/telemetry/... -v` output — spec-prescribed and accepted.

§2 decisions 1–5 (mirror exclusion shape, empty-string signal, no config toggle, synchronous timing, two-test strategy) ack. §6: no open questions — ack.

Address pin 1 (design_decisions in status.json) inline before transitioning to in_progress, then proceed.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Single mechanical pin (populate design_decisions in status.json — recurring T12 pattern, apply-inline before coding); design is correct against live code, all ACs covered, no cross-slice collisions.
-->
