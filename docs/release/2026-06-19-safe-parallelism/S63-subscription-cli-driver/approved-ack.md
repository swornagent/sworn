<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Design is sound — subprocess driver approach is correct and well-scoped. 9 pins + 3 flags, all mechanical inline fixes:

1. **Add provider.go to planned_files.** `internal/model/provider.go` is in design §3 but missing from status.json planned_files. Add it — Gate 2 will fail without it (7th recurrence of this pattern).

2. **Add design_decisions to status.json.** Add a `design_decisions` array with the 5 §2 decisions, all classified Type-2 (local, reversible). Designfit gate fails closed without it.

3. **Codex case in NewClient returns error, not cliDriver.** §2 says both claude-cli and codex return *cliDriver, but §4 defers codex. The codex case should return a deferral error (ErrDriverNotRegistered or typed model.Error with "codex support deferred (S63-deferral-1)"). Update §2 decision 2 accordingly.

4. **CRITICAL: Bypass proxy routing for claude-cli/codex in FromEnv.** config.go L52-78 runs proxy routing BEFORE the keyless switch at L87-108. A sworn-logged-in user configuring claude-cli will get proxy-routed to OAI, never reaching the subprocess driver. Add an early return for claude-cli/codex providers BEFORE the proxy routing block — call NewClient() directly and return.

5. **Map missing binary to KindOther, not KindTransient.** exec.ErrNotFound → KindTransient means the caller retries, but a missing binary is permanent. Use KindOther (terminal) instead.

6. **CRITICAL: Include --no-session-persistence in claude -p invocation.** Without it, session state may persist between Verify calls, violating the fresh-context property (Rule 7). Also include --model <model> to pass the user-chosen model. The invocation should be: `claude -p --no-session-persistence --model <model> <prompt>`.

7. **Specify how systemPrompt + userPayload become the claude -p arg.** Concatenate systemPrompt + "\n\n" + userPayload as the single prompt argument to `claude -p`.

8. **File GitHub issue for codex deferral.** Create the issue during implementation, cite the number in journal.md and the TODO comment. The why (different invocation/normalisation) and acknowledgement (journal.md) are present; tracking needs the issue number.

9. **Use slash syntax consistently.** parseModelID splits on `/`. Use `claude-cli/sonnet` not `claude-cli:sonnet`. The spec's colon notation is advisory.

Flags (not pins): (a) costUSD will always be 0 with plain text capture — state this in a code comment; (b) --model flag is required to honour per-role config — included in pin 6; (c) non-zero exit → KindAuth is coarse but acceptable for v1.

§2 decisions 1-5 ack (all Type-2, well-rationaled). §6 questions empty — ack.

Address pins 1-9 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 9 pins are apply-inline mechanical fixes (missing file in planned_files, missing design_decisions, proxy routing bypass, error kind correction, flag addition, prompt assembly, deferral tracking, syntax consistency); no human judgement required.
-->
