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
