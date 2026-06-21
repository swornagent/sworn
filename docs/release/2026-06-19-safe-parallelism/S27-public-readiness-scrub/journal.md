---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S27-public-readiness-scrub`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to make the sworn repo + binary public-safe before
launch. Splits off the scrub work that does NOT fit S21's embed scope: generalising the
embedded role prompts, removing dogfood provenance comments, and clearing the
fired/GetFired + coach-loop references across source and release artefacts.

Decision (brad, 2026-06-21): **keep** the sport-aligned role vocabulary (Captain /
Coach / Planner / Implementer / Verifier) — the scrub strips the private-orchestration
*coupling*, not the role *names*. Placed in its own track `T10-public-readiness`
depending on every other track, so it runs last (collision-free) and acts as the launch
gate: sworn must not go public until this slice is `verified`.

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

*(None yet.)*
