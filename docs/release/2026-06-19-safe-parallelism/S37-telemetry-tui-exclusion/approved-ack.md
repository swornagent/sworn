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
