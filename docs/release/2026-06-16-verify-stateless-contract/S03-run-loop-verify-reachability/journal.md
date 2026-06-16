# Journal — S03-run-loop-verify-reachability

## Session 2026-06-17 — Implementation

**State transition**: `planned` → `in_progress` → `implemented`

### Decisions

- **No injection seam needed.** `run.Run` already exposes `Options.NewVerifier`
  as a factory for `model.Verifier`, plus `Options.NewAgent` for the implementer
  side. The existing `TestRun_PassPath_Merges` already exercises the run loop
  with a `fakeVerifier`. The new tests add a `textVerifier` (raw-text reply,
  optional system prompt capture) and wire it through the same injection point.
  Zero production code changes.

- **Three tests, not one.** The spec's three acceptance checks (markdown PASS,
  stateless prompt wiring, tool-call-leak block) are three distinct integration
  tests — each drives `run.Run` with a specific fake verifier and asserts the
  run-loop outcome. Reuses `stdoutAgent` from the existing test harness.

- **`textVerifier` is a separate type, not an extension of `fakeVerifier`.**
  The existing `fakeVerifier` is scripted from `verdict.Result` objects and
  emits `"PASS: <rationale>"` text. The new tests need raw, arbitrary reply
  text (e.g. `"**PASS**"`, `<tool_call>...</tool_call>`) and optional system
  prompt capture. A separate minimal type avoids disturbing the existing tests.

- **Multi-provider manual reachability (AC4):** Three providers tested with a
  synthetic spec+diff (add function):
  - Deepseek (deepseek-chat): PASS, exit 0
  - Groq (llama-3.1-8b-instant): PASS, exit 0 (returned `**PASS**` w/ markdown)
  - Google (gemini-2.5-flash): PASS, exit 0
  - Deepseek (deepseek-chat) with broken diff (subtract): FAIL, exit 1
  - Deepseek (deepseek-chat) with ambiguous spec: BLOCKED, exit 2
  - Gemimi 2.0-flash dispatch failure: BLOCKED, exit 2
  No `unparseable_verdict` observed. Groq's `**PASS**` proved the tolerant
  parser end-to-end at the run-loop level.

### Trade-offs

- INCONCLUSIVE not triggered — the three models all rendered determinate
  verdicts on the synthetic inputs. The parser handles it (code path exists,
  exit code 3), but no live model returned it. The spec's claim is "returns a
  parseable PASS/FAIL/BLOCKED/INCONCLUSIVE" — absence of INCONCLUSIVE on these
  inputs is not a format-variance failure.

### Out-of-scope discoveries

None.