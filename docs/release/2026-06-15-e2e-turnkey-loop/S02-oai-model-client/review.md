# Captain review — S02-oai-model-client
Date: 2026-06-16
Captain version: 0.1
Design TL;DR commit: 9c7a812754241cb5320188d960241639756c44b2

## Pins

1. [mechanical] §1 — AC3 "never logged" is not explicitly committed
   What I observed: spec AC3 says "Provider key is read from env (BYO-key); **never logged**." §1 covers key-from-env + BLOCK-on-missing, but never says the key/request-body won't be logged. The repo's own AGENTS.md Security rule also states "Never log API keys, request bodies, or model payloads."
   What to ask the implementer: add an explicit line in §1 or §4: "API keys and request/response bodies are never logged (per AGENTS.md Security)." Then honour it in code (no `fmt.Printf` / `log.Printf` of key or payload).

2. [mechanical] §1 — AC4 "HTTP/timeout → BLOCKED" is implicit
   What I observed: spec AC4 says "An HTTP/timeout error → BLOCKED (fail-closed), not a crash or false PASS." §1 says "same fail-closed contract as today" without enumerating the error paths. The test plan in §3 covers HTTP 500 and timeout, so the coverage exists — but the design commitment is implicit.
   What to ask the implementer: confirm in §1 that the OAI client returns errors (not panics) on HTTP failures, and that `verify.Run` → `v.Verify()` → err path maps to BLOCKED (which it already does — verify.go:57-58). A one-line confirmation suffices.

3. [mechanical] §2 — Spec Risk #2 "never log keys/payloads" has no matching decision
   What I observed: spec Risks section says "Key/payload leakage in logs — never log request bodies or keys." No §2 decision addresses logging discipline. The AGENTS.md Security rule already mandates this, but the design should cite it.
   What to ask the implementer: add a §2 decision (or amend Decision 2/4) stating "No logging of API keys, request bodies, or response payloads (per project AGENTS.md Security rule)." Then honour it — no `fmt.Printf`/`log.Printf` of key, body, or response JSON.

4. [mechanical] §2 — Spec Risk #1 "normalise; fail closed on unrecognised shape" has no normalisation strategy
   What I observed: spec Risk #1 says "Provider response-shape variance — normalise; fail closed on unrecognised shape." The test plan covers garbled JSON and missing `usage` block, but the design doesn't describe a normalisation strategy for the OAI response shape.
   What to ask the implementer: confirm the JSON unmarshalling strategy: (a) `json.Decode` into a struct with only the fields you need (ignoring unknowns — this is the normalisation), (b) if `usage` block is missing → zero cost (Decision 4 already covers this), (c) any other unrecognised shape that prevents unmarshalling → fail closed (return error, which verify.go maps to BLOCKED). Document this in the design or in a code comment on the OAI struct.

5. [mechanical] §3 — Empty model flag guard in main.go
   What I observed: design says "Nil/empty model flag stays Unconfigured (backward-compatible)" but the §3 code edit description only says "replace the `// Verifier left nil` line with `model.FromEnv(*mdl)`." It doesn't describe the guard condition.
   What to ask the implementer: guard the `model.FromEnv(*mdl)` call behind `if *mdl != ""` — when `--verifier-model` is empty/unset, leave `Verifier` nil so the existing `Unconfigured` fallback in verify.go:53-55 fires. This is a one-line `if`.

6. [mechanical] §3 vs spec — Spec `planned_files` lists `internal/verify/verify.go` but design doesn't touch it
   What I observed: spec.md's "Planned touchpoints" lists `internal/verify/verify.go`, but design.md §3 does not include it. The design is correct — verify.go already wires the `Verifier` interface; no changes needed for S02.
   What to ask the implementer: either update spec.md's planned touchpoints to remove `internal/verify/verify.go`, or add a brief note in design.md §4 explaining why it's intentionally not touched. This prevents S04's implementer from seeing a stale touchpoint and assuming S02 modifies verify.go.

## Summary

Pins: 6 total — 6 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none — all are apply-inline confirmation/doc fixes; the existing code path in verify.go already handles errors → BLOCKED correctly.

## Smaller flags (not pins, worth one-line ack)

- (a) §6 is empty — implementer found no open questions. Clean but worth noting.
- (b) No project memory for `sworn` exists yet. As the repo matures, memory entries for provider conventions, env var naming patterns, and cost tracking should be created.
- (c) `provider/model` slash separator (Decision 1): model IDs that themselves contain slashes (rare but possible) would be ambiguous. Not an issue today with `openai/gpt-4.1` but worth flagging for S10 when the benchmark picks a default.

## Suggested ack reply

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