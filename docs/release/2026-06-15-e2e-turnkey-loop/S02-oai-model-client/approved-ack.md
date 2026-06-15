TL;DR clean design — the OAI client is well-scoped, all spec ACs are covered, and the file plan is minimal. 6 mechanical pins + 3 flags:

1. **AC3 "never logged".** Add an explicit line in §1 or §4 committing to never log API keys, request bodies, or response payloads (per AGENTS.md Security).
2. **AC4 HTTP/timeout → BLOCKED.** Confirm in §1 that the OAI client returns errors (not panics) on HTTP failures. The existing verify.go:57-58 already maps err → BLOCKED — just state it.
3. **Risk #2 "no logging".** Add a §2 decision citing the AGENTS.md Security rule: no logging of keys/body/response.
4. **Risk #1 "normalise; fail closed".** Document the normalisation strategy: `json.Decode` into a struct with only needed fields (ignoring unknowns), missing `usage` → zero cost, unparseable → return error (→ BLOCKED).
5. **Empty model flag guard.** Guard `model.FromEnv(*mdl)` with `if *mdl != ""` in main.go so the nil-Verifier→Unconfigured fallback still works.
6. **Spec planned_files.** Either remove `internal/verify/verify.go` from spec's planned touchpoints or add a §4 note explaining why S02 doesn't touch it (S04 will).

Flags (not pins): (a) §6 is empty — clean; (b) no project memory yet for sworn — file one post-slice; (c) `provider/model` slash separator may be ambiguous if model IDs contain slashes — not an issue today, flag for S10.

§2 decisions 1-5 ack. §6 ack (empty).

Address pins 1–6 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: all 6 pins are mechanical doc/clarification fixes apply-inline during implementation; no design re-review needed; existing verify.go error→BLOCKED path already carries AC4
-->
