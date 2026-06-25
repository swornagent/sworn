# Captain review — S50-baton-governance
Date: 2026-07-07
Design commit: fa7b4a64c5a78b3f64987dea1f709c58268b2ad2

## Pins

1. [mechanical] §2b / status.json — design_decisions field is missing from status.json
   What I observed: S50's status.json has no `design_decisions` array. S50's `planned_files` include `cmd/sworn/baton.go`, which is under the `cmd/sworn/` architecturally-significant prefix in the designfit gate (`internal/designfit/designfit.go:79-100`). The gate will fail closed: "implies Type-1 work (planned_files touch architecturally-significant packages) but design_decisions is empty."
   What to ask the implementer: Add `design_decisions` to status.json with the 5 decisions from design.md §2, all classified Type-2 (they are narrow, reversible, follow existing conventions). Do this before transitioning to in_progress.

2. [mechanical] §4 — live-remote diff deferral tracking is vague
   What I observed: §4 defers live-remote diff with tracking = "future slice / sawy3r/baton issue" — no specific issue number. The CI wiring deferral correctly names S50 proof.md as tracking; the upstream PR deferral correctly names sawy3r/baton#31.
   What to ask the implementer: Replace "future slice / sawy3r/baton issue" with a concrete tracking reference. S62-baton-upstream-source is the planned slice that will deliver network fetch; cite it by slice-id, or file a baton issue and cite the number.

## Summary
Pins: 2 total — 2 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none

## Smaller flags (not pins, worth one-line ack)
- (a) Risk #3 says "don't let docs/baton-governance.md duplicate ADR-0006 prose." The design says the doc "links" the ADR but doesn't explicitly state "link, don't duplicate." The implementer should keep the governance doc as an operational how-to (steps + links) and avoid restating ADR-0006's decision rationale.
- (b) The governance doc will live in the public sworn repo and link sawy3r/baton#31 (public). Per [[feedback_public_repo_leakage_check]], ensure no private repo refs (private project refs, slice IDs from other releases, release codenames) appear in the doc body.
- (c) `sworn baton vendor --check` already does a dry-run diff (prints transform diff without writing). `sworn baton diff` is a separate subcommand that compares committed embed vs transformed source and exits non-zero on divergence. The implementer should ensure the two don't confuse users — `diff` is the governance/fail-closed surface; `vendor --check` is the developer dry-run. Consider whether the help text distinguishes them clearly.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR design is sound — 5 Type-2 decisions, shared code paths verified, no spec drift. 2 pins + 3 flags:

1. **Add design_decisions to status.json.** S50 touches `cmd/sworn/baton.go` (architecturally-significant prefix). The designfit gate fails closed when `design_decisions` is empty and `planned_files` include `cmd/sworn/`. Add the 5 decisions from §2 as Type-2 entries before transitioning to in_progress.

2. **Concretise live-remote diff deferral tracking.** §4 says "future slice / sawy3r/baton issue" — replace with a specific reference. S62-baton-upstream-source is the planned slice for network fetch; cite it by slice-id.

Flags (not pins): (a) governance doc should link ADR-0006, not duplicate its prose; (b) scrub governance doc for private repo refs per [[feedback_public_repo_leakage_check]] before commit; (c) distinguish `sworn baton diff` (governance, fail-closed) from `sworn baton vendor --check` (developer dry-run) in help text.

§2 decisions 1-5 all Type-2, ack. §6 questions: none, ack.

Address pins 1-2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Two mechanical pins (missing design_decisions field, vague deferral tracking) both fixable inline during implementation; no design re-check needed.
-->