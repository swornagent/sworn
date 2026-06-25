<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Clean design, 2 mechanical pins to address inline:

1. **AC4 evidence in proof.** The `verify --help` step in §5 is read-only — it won't produce the state-write evidence AC4 requires. In proof.md, replace or supplement it with a state-writing invocation from the repo root (confirm `.sworn/sworn.db` lands at repo root), OR add a code citation to the CWD-relative state-dir resolution in `internal/config/` so the Verifier can confirm structurally.
2. **`design_decisions` in status.json.** Populate the `design_decisions` field with the 5 §2 decisions (all Type-2; no `human_decision` field needed). Use S38's entry format as reference. `sworn designfit` currently passes (non-arch-sig files), but the field should be populated consistently.

Flags (not pins): (a) S33 deferral for prompt smoke-step wording is orphaned — filed as GH #9, no action needed from S41; (b) 1 unmerged release-wt commit (T17 replan) — doesn't affect S41 artefacts.

§2 decisions (D1–D5) are all verified correct from live repo (Makefile confirmed to have build/test/vet/fmt/clean + ldflags; CI confirmed to use go vet/test directly; docs/build.md correctly targeted as new file). §6 empty — ack.

Address pins 1–2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both pins are apply-inline corrections (reachability evidence swap + status.json field population); no Coach authority needed and no design change required.
-->
