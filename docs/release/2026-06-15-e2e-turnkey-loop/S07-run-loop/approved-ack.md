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
