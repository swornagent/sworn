# Captain review — S07-run-loop
Date: 2026-06-16
Captain version: 0.1
Design TL;DR commit: e60c7f2bce7d1e8fa1e561de8b0576e92247e868

## Pins

1. [mechanical] §5 — Reachability plan only covers `internal/run/run_test.go` (Go package-level tests). The CLI entry point `sworn run` (`cmd/sworn/run.go`) is not exercised. Rule 1 (Reachability Gate) requires the first failing test to render through the integration point that owns the affordance — for a CLI, that means invoking the binary or its dispatch path.
   What I observed: §5 lists three test scenarios (PASS, FAIL, FAIL-then-PASS) all at `internal/run/run_test.go` with fake agents. The spec's "Required tests" says "Integration: fake implementer + verifier models scripted" but the reachability artefact should exercise `sworn run`, not just the orchestration engine in isolation.
   What to ask the implementer: Add at minimum a smoke step or CLI-level integration test that invokes `sworn run --task "..."` and asserts exit code / merge outcome. The internal/run tests are the engine proof; the CLI test is the reachability proof.

2. [mechanical] §2 + §1 — State transition gap: the design says "the merge gate checks `state == verified` directly" but does not specify who transitions state from `implemented` → `verified`. The `verify.Run()` function returns a `verdict.Result` but does not write status.json. The run loop must perform this transition before merging.
   What I observed: `internal/verify/verify.go`'s `Run()` returns a `verdict.Result` struct; it never calls `state.Write()`. The design's integration test says "assert state is `verified`" post-PASS, implying the run loop transitions state. But §1 and §2 never mention this step.
   What to ask the implementer: After `verify.Run()` returns PASS, the run loop must call `st.State.Transition(state.Verified)` and `state.Write()` before attempting the merge. State this explicitly in the implementation and in the test assertions.

3. [mechanical] §2 Decision 1 — Auto-generated spec.md and status.json format unspecified. The design says the run loop creates "a minimal slice directory with an auto-generated spec.md and status.json" but does not define the template.
   What I observed: `implement.Run()`'s `extractScope()` looks for a `## User outcome` heading in the spec. If the auto-generated spec lacks this heading, scope extraction degrades to "No scope found in spec." `state.Status` has ~15 fields; the run loop must populate at minimum `slice_id`, `release`, `state`, `spec_path`, `proof_path`, `release_base`.
   What to ask the implementer: Define the auto-generation template before writing code. Minimal viable: spec.md = `# Task\n\n<user's description>` (or include `## User outcome` if scope extraction matters); status.json = `state.Status` with the required fields set. Confirm with a smoke test that `implement.Run()` can read the auto-generated artefacts.

4. [mechanical] §3 — `cmd/sworn/main.go` touchpoint collision with S08-init-config (T3-turnkey-ux, state: verified). S08 adds an `"init"` case to the dispatch switch; S07 adds a `"run"` case. Both are additive to the same shared file.
   What I observed: S08's `status.json` lists `cmd/sworn/main.go` in `planned_files`. S07's design §3 also lists `cmd/sworn/main.go`. Both add non-overlapping switch cases. T3 is on a separate track; the collision resolves at merge time.
   What to ask the implementer: Acknowledge the touchpoint collision in a code comment near the switch. Ensure the `"run"` case is added cleanly after the existing `"verify"` and `"init"` cases. No sequencing gate needed — this is a standard additive dispatch pattern.

5. [mechanical] §1 — Model escalation tier names ("nano → mini → 4.1 → 4o → o3-mini → o3") are shorthand, not standard model identifiers. The design does not map these to actual API model IDs or make the escalation path user-configurable.
   What I observed: `internal/model/oai.go`'s `FromEnv()` takes a model ID string and passes it as `"model"` in the API request. Shorthand names like "nano" or "4.1" are not valid OpenAI model IDs. The implementer must either (a) define a mapping from tier names to actual model IDs, or (b) make the escalation path configurable via a flag/env var with actual model IDs, or (c) hardcode real model IDs.
   What to ask the implementer: Define the model ID mapping before implementation. If the escalation path is hardcoded, use real model identifiers (e.g., `gpt-4o-mini`, `gpt-4o`, `o3-mini`, `o3`). If configurable, add an `--escalation-models` flag or `SWORN_ESCALATION_MODELS` env var. Document the mapping in the `--help` output.

## Summary

Pins: 5 total — 5 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): None. All pins are apply-inline clarifications; none would cause the slice to ship broken if addressed during implementation.

## Smaller flags (not pins, worth one-line ack)

(a) No project memory index exists at `~/.claude/projects/-home-brad-projects-sworn/memory/`. Memory cross-reference (§2) was unavailable for this review. Consider seeding one after this slice lands.

(b) §6 is empty despite Decision 1 (auto-generated release structure + spec.md from task string) being a significant design choice with non-trivial implementation implications. The design is coherent but the absence of open questions is notable — confirm the implementer has fully resolved the auto-generation format.

(c) `internal/git/git.go` has no `Merge()` function. The run loop needs merge capability (checkout base branch, merge feature branch). The implementer can use `exec.Command("git", "merge", ...)` directly or extend the git package. Either is fine; just don't forget it.

## Suggested ack reply

TL;DR Clean design — orchestration engine with clear AC coverage. 5 mechanical pins + 3 flags:

1. **Reachability: add CLI-level test.** The internal/run tests cover the engine; add a smoke step or test that invokes `sworn run` and asserts exit code / merge outcome.
2. **State transition: implemented→verified.** After `verify.Run()` returns PASS, call `st.State.Transition(state.Verified)` + `state.Write()` before merging. State this explicitly.
3. **Auto-generated spec/status format.** Define the template before writing code. Minimal: spec.md = `# Task\n\n<description>`; status.json with slice_id, release, state, spec_path, proof_path, release_base populated. Verify with a smoke test that implement.Run() can read the generated artefacts.
4. **main.go touchpoint with S08.** Acknowledge in a comment near the switch. Both "init" and "run" cases land cleanly.
5. **Model escalation tier names → real IDs.** Map "nano"/"mini"/"4.1"/"4o"/"o3-mini"/"o3" to actual model identifiers or make the escalation path configurable. Document the mapping in `--help`.

Flags: (a) no project memory index; (b) §6 empty despite auto-generation decision; (c) git.Merge() capability needed — extend package or use exec.Command.

§2 decisions 1–5 ack. §6 empty ack.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are apply-inline mechanical clarifications (CLI reachability test, state-transition step, template format, touchpoint collision, model-ID mapping); no design re-review needed.
-->