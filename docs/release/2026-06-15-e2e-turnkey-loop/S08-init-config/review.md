# Captain review — S08-init-config
Date: 2026-06-15T20:27:38Z
Captain version: 0.1
Design TL;DR commit: fcc84731cda980b8c9a50b1de80c9b84cc5c6fcd

## Pins

1. [mechanical] §1 vs AC1 — cross-slice AC narrowing
   What I observed: Spec AC1 says "After `sworn init` + one key, `sworn run` works with defaults." Design §1 frames the outcome as "After init, `sworn verify` can resolve a verifier model from config." §4 explicitly defers `sworn run` integration to S07: "`sworn run` integration — S07 owns wiring config into the full loop."
   What to ask the implementer: Confirm the Coach accepts that AC1 is partially delivered by S08 (config infra + init) and final verification happens when S07 lands. This is a cross-slice AC that can't close on S08 alone.

2. [mechanical] §2 vs AC4 — config idempotency unspecified
   What I observed: Spec AC4 requires "`sworn init` is idempotent." §2.4 details adoption splice idempotency (section-level replacement, byte-level compare, no-op on identical content), but the config file scaffold behaviour on re-run is unspecified. If `config.json` already exists, does `sworn init` overwrite it? Skip? Prompt the user?
   What to ask the implementer: Specify config file idempotency behaviour. Reasonable options: (a) skip if config exists and print "config already exists at <path>", (b) prompt to overwrite, (c) merge (harder). Pick one and document in §2.

3. [mechanical] §1/§4 vs AC3 — missing-key error path underspecified
   What I observed: AC3 requires "A missing key produces a clear, actionable error (not a crash or false PASS)." Design §2.3 covers key storage (0600 permissions, warning about key-in-file risk), but the error UX when a key is missing is not described. §4 says "Provider API key validation — the error surfaces naturally on first `sworn verify`" — this is an inference, not a designed path.
   What to ask the implementer: Design the missing-key error path explicitly. What does `sworn verify` say when no key is configured? Is it a BLOCKED verdict with a message like "no verifier model configured — run `sworn init` first"? The "surfaces naturally" assumption needs verification against the actual `model.FromEnv` error output.

4. [mechanical] §2 vs spec Risk — config location docs missing
   What I observed: Spec Risk mitigation says "never log keys; document config location." The design addresses key leakage (0600 permissions + warning) but the second half — "document config location" — has no corresponding deliverable in §3. No README update, no `sworn help` line, no inline comment.
   What to ask the implementer: Add a config-location documentation surface. Minimal: a line in `sworn help` output listing the config path. Or a line in README.md. The spec explicitly requires it.

5. [mechanical] §5 reachability — CLI smoke uses hardcoded key only
   What I observed: §5's CLI smoke test uses `--api-key test-key-123`. This tests the happy path but doesn't exercise the missing-key error path (AC3). The unit tests mention "missing-key error path" but the reachability plan doesn't show a smoke test for the error case.
   What to ask the implementer: Add a smoke test for the missing-key error path — run `sworn verify` without a key and confirm the output is clear and actionable (AC3).

6. [mechanical] §3 adoption source gap — AGENTS.md seven-rule fragment source unspecified
   What I observed: §3 says `Materialise` writes `docs/baton/` "rules + VERSION from embedded content." Decision 5 pins the VERSION source (`prompt.BatonVersion()`), and the role prompt files (verifier.md, implementer.md, planner.md, captain.md) are embedded in `internal/prompt/` — those are the "rules" to vendored into `docs/baton/`. That part is clear. However, the **seven-rule fragment** that `SpliceAgents` inserts into `AGENTS.md` has no specified source. It's not in the S04 embed, and Decision 5 explicitly rules out a separate `go:embed` of `docs/baton/`. The fragment text (the `## Engineering Process — Baton` section with the seven rules) must come from somewhere — a Go string constant? Read from the binary's own embedded content?
   What to ask the implementer: Specify the source of the seven-rule AGENTS.md fragment. Options: (a) hardcode it as a Go string constant in `internal/adopt/` (simple, works), (b) add it to the `internal/prompt/` embed (adds a new file to S04's surface), (c) read it from the project's own AGENTS.md at build time. Pick one and document in §3.

7. [mechanical] §3 touchpoint — `cmd/sworn/main.go` shared with S07
   What I observed: S07 (planned, T2-orchestration) also touches `cmd/sworn/main.go` to add `case "run":`. S08 adds `case "init":`. Both are new case entries in the same switch — low-risk merge, but the insertion point could conflict. S01 and S04 (verified, T1-engine) already landed their main.go edits.
   What to ask the implementer: Choose a consistent insertion convention (e.g., alphabetical case order: `init` before `run`, or append-new-at-bottom). Coordinate with S07 implementer or pick an insertion order that makes the merge trivial.

## Summary
Pins: 7 total — 7 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): 2 (config idempotency — user could lose config on re-run), 3 (missing-key error UX — could ship crash/opaque error violating AC3), 6 (AGENTS.md fragment source unspecified — blocks SpliceAgents implementation)

## Smaller flags (not pins, worth one-line ack)

(a) Decision 5 says adoption uses `prompt.BatonVersion()` not a separate embed — this is clean, but confirm the `BatonVersion()` string format (currently "v1.0.0") is the exact format expected in `docs/baton/VERSION`.
(b) §3 `internal/config/config.go` lists `ModelConfig` per-role model selections — consider whether the config struct should use the same role names as the embedded prompts (verifier, implementer, planner, captain) for consistency.

## Suggested ack reply

TL;DR clean design, well-scoped. 7 pins + 2 flags:

1. **AC1 narrowing.** AC1 is cross-slice (depends on S07). Confirm S08 delivers config infra + init; AC1 closes when S07 lands.
2. **Config idempotency.** What happens when config.json exists? Pick skip-with-message, prompt-to-overwrite, or merge. Document in §2.
3. **Missing-key error UX.** Design the error path: what does `sworn verify` output when no key is configured? Don't rely on "surfaces naturally" inference.
4. **Config location docs.** Spec Risk requires documenting config location. Add to `sworn help` or README.
5. **Smoke test missing-key path.** Add a smoke test exercising the no-key error case (not just the happy path with `--api-key test-key-123`).
6. **AGENTS.md fragment source.** Where does the seven-rule fragment text come from? Hardcode as Go constant, add to `internal/prompt/` embed, or read from project AGENTS.md. Document in §3.
7. **main.go merge with S07.** Both add new case entries to same switch. Pick insertion convention (alphabetical: `init` before `run`) for trivial merge.

Flags (not pins): (a) confirm BatonVersion() format matches expected docs/baton/VERSION content; (b) consider role-name consistency between config.ModelConfig keys and embedded prompt role names.

§2 decisions 1–5 ack. §6 empty ack.

Address pins 1–7 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 7 pins are apply-inline mechanical corrections (source gaps, unspecified behaviours, missing smoke test); none requires design re-review or Coach authority.
-->