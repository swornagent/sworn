# Captain review — S41-build-bin-target
Date: 2026-06-23
Design commit: 7e6953828b1eef4543e503e542b0f183eff408ec

## Pins

1. [mechanical] §5 — `verify --help` doesn't demonstrate AC4
   What I observed: The §5 reachability plan's third step runs `./bin/sworn verify --help && ls -d .sworn/ 2>/dev/null || echo "no state written for read-only verify"`. The design's own echo confirms this is a read-only invocation. A `--help` flag writes no state; the step proves that help doesn't write state, not that state-writing invocations land at the repo root (AC4: "Running `./bin/sworn <a command that writes state>` from the repo root writes `.sworn/` / run-scratch at the repo root, **not** under `cmd/sworn/`"). The Verifier checking AC4 will have no runtime evidence from this step.
   What to ask the implementer: Replace the third proof step with a state-writing invocation run from the repo root, then confirm `.sworn/sworn.db` (or equivalent) appears at the repo root. If a live run is impractical in the proof environment, add a code citation pointing to the state-dir resolution in `internal/config/` that shows CWD-relative writes — then the Verifier can confirm the claim structurally.

2. [mechanical] status.json — `design_decisions` field absent
   What I observed: `S41-build-bin-target/status.json` has no `design_decisions` key. The design.md §2 lists 5 decisions; none are recorded in status.json. `sworn designfit` passes (Makefile and docs/build.md don't touch `cmd/sworn/`, `internal/state/`, or `internal/verdict/`, so `impliesType1Work()` returns false), but the field should be populated per the harness pattern established in S38 and every T12 slice that preceded S41. This is the 5th consecutive T12 slice where this gap has appeared (S35/S36/S37/S38/S41 — trial log has all four prior instances).
   What to ask the implementer: Populate `design_decisions` in status.json with the 5 §2 decisions (all Type-2, no `human_decision` needed). Use the S38 entries as the format reference.

## Summary
Pins: 2 total — 2 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none — both pins are apply-inline and would not cause the slice to ship broken (the build tooling and doc work are correct; the Verifier might BLOCK on Pin 1 if AC4 is not separately provable, but the underlying behaviour is sound).

## Smaller flags (not pins, worth one-line ack)

- **Stale deferral to S33 for prompt smoke-step wording.** The spec's Out-of-scope section says: "reachability smoke-step prompt wording deferred to S33-spec-template-hardening." S33 is now verified but did NOT add any `make build`/`./bin/sworn from root` wording to `internal/prompt/*.md` (confirmed by grep). The deferred work has no tracking. Filed as GH issue **#9** to give it a new home. S41 does not need to pick this up — it's outside S41's spec scope.
- **Drift gate: 1 commit in release-wt not in track.** Commit `e041b55` (replan R3 — add T17-orchestration-core) is in release-wt but not in the track. The track has the equivalent content via its own replan commit `567b8f1` + merge `85dccf7`. S41's artefacts are unaffected; no drift in spec content.

## Suggested ack reply
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
