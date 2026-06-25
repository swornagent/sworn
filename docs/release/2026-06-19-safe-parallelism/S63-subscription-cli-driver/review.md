# Captain review — S63-subscription-cli-driver
Date: 2026-07-12
Design commit: 78e298e3dbe3641cb878591bcf06cfa127825eab

## Pins

1. [mechanical] §3 — provider.go missing from status.json planned_files
   What I observed: design.md §3 correctly lists `internal/model/provider.go` (adds claude-cli and codex cases to NewClient() switch), but status.json planned_files is `['internal/model/cli.go', 'internal/model/config.go', 'internal/model/cli_test.go']` — provider.go absent. Same recurring pattern as S12/S13/S14/S15/S39 (7th occurrence in trial log).
   What to ask the implementer: Add `internal/model/provider.go` to status.json planned_files before code. Gate 2 will fail without it.

2. [mechanical] §2 — design_decisions missing from status.json
   What I observed: status.json has no `design_decisions` field. The design has 5 decisions in §2, all with rationale. The designfit gate (Rule 9) fails closed on missing design_decisions. 11th recurrence in trial log.
   What to ask the implementer: Add a `design_decisions` array to status.json with the 5 §2 decisions, each classified Type-1 or Type-2. Decisions 1-5 are all Type-2 (easy to reverse, narrow/local — driver struct, provider prefix, keyless bypass, timeout config, env var override).

3. [mechanical] §2/§4 — codex case in NewClient contradicts codex deferral
   What I observed: §2 decision 2 says "NewClient() adds `claude-cli` and `codex` provider prefixes, returning a `*cliDriver`." §4 says "Not implementing `codex exec` — CLAIM AS RULE 2 DEFERRAL." If codex is deferred, the codex case in NewClient cannot return a working *cliDriver.
   What to ask the implementer: The codex case in NewClient should return an error (e.g. `ErrDriverNotRegistered` or a typed `model.Error{Kind: KindOther}` with a "codex support deferred (S63-deferral-1)" message). Update §2 decision 2 to say only claude-cli returns *cliDriver; codex returns a deferral error. This makes codex "selectable" (AC2) but non-functional, consistent with the spec's "Deferrals allowed?" section.

4. [mechanical] §2/§5 — CRITICAL: proxy routing in FromEnv intercepts claude-cli before keyless check
   What I observed: design.md §2 decision 3 says "FromEnv() treats claude-cli and codex as keyless providers — no API key check. The switch in the key-gate section (L87-108) adds them alongside vertex, bedrock, and oci." But config.go L52-78 runs proxy routing FIRST: if sworn login credentials are present and SWORN_DIRECT is not set, FromEnv returns an &OAI{} with the proxy URL — the keyless switch at L87-108 is never reached. A user who is logged in to sworn AND configures claude-cli will get proxy-routed, never reaching the subprocess driver.
   What to ask the implementer: Add claude-cli/codex bypass BEFORE the proxy routing check in FromEnv. After parseModelID, check if provider is "claude-cli" or "codex" and return early via NewClient() — skip the proxy routing block entirely. This ensures the subprocess driver is selected regardless of sworn login state.

5. [mechanical] §4 — missing binary classified as KindTransient is semantically wrong
   What I observed: §4 says "the driver classifies `exec.ErrNotFound` as `KindTransient` (missing binary)." But `IsTransient` returns true for KindTransient (not terminal), meaning the caller will retry. A missing binary will never succeed on retry — it is a permanent condition, not transient.
   What to ask the implementer: Map exec.ErrNotFound to `KindOther` (unclassified, treated as terminal by IsTerminal since it's not Auth/Credits) or introduce a new kind. KindTransient is for conditions that may succeed on retry (rate limits, upstream 5xx). A missing binary is permanent.

6. [mechanical] §4 — --no-session-persistence flag not mentioned in design
   What I observed: The spec says `claude -p <prompt> [--output-format json] [--no-session-persistence]`. The design §4 says "capture stdout as the verdict text. No tool loop — `claude -p` with the full role prompt as the arg returns text." The design does not mention --no-session-persistence. Without this flag, `claude -p` may persist session state between calls, violating the fresh-context property that the Verifier interface guarantees (Rule 7). The reference driver (claude-cli.sh) always uses --no-session-persistence.
   What to ask the implementer: Include `--no-session-persistence` in the claude -p invocation. This is load-bearing for the adversarial verification property — the Verifier interface contract requires each call to be fresh-context. Also include `--model <model>` to pass the user-chosen model.

7. [mechanical] §2/§4 — design doesn't specify how Verify's systemPrompt + userPayload map to claude -p invocation
   What I observed: The Verifier interface is `Verify(ctx, systemPrompt, userPayload string) (text, costUSD, err)`. Existing drivers (OAI, Anthropic) pass systemPrompt and userPayload as separate system/user messages. `claude -p` takes a single prompt argument. The design says "the full role prompt as the arg" but doesn't specify how the two strings are combined into one arg.
   What to ask the implementer: Specify the prompt assembly: concatenate systemPrompt + "\n\n" + userPayload as the single arg to `claude -p`, or use `--system-prompt` if the CLI supports it. The concatenation approach is simpler and matches the reference driver's behaviour (prompt as single arg).

8. [mechanical] §4 — codex deferral tracking incomplete
   What I observed: §4 says "a GitHub issue" but no issue exists (gh issue list --search codex returns no match). Per Rule 2, tracking must be a linked issue, plan task, or punch-list item. The design says `// TODO: codex exec support (S63-deferral-1)` which is a slice-id reference (acceptable), but the GitHub issue must be filed.
   What to ask the implementer: File a GitHub issue for codex exec support during implementation. Cite the issue number in journal.md and in the TODO comment. The deferral's "why" (different invocation/normalisation) and "acknowledgement" (journal.md) are present; tracking just needs the issue number.

9. [mechanical] §1/§5 — spec uses colon syntax `claude-cli[:<model>]` but existing code uses slash `provider/model`
   What I observed: The spec says `claude-cli[:<model>]` (colon). The design §1 uses `claude-cli:<model>` (colon). The design §5 uses `claude-cli/sonnet` (slash). The existing `parseModelID` function splits on `/` (slash). The design has silently normalised to slash in §5 without acknowledging the spec's colon notation.
   What to ask the implementer: Use slash syntax (`claude-cli/sonnet`) consistently — it's what parseModelID expects. The spec's `[:<model>]` notation is advisory format, not a literal API contract. Ensure §1 and §5 are consistent.

## Summary

Pins: 9 total — 9 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: 4 (proxy routing intercepts claude-cli; --no-session-persistence missing; missing binary wrong Kind; provider.go missing from planned_files)

## Smaller flags (not pins, worth one-line ack)

- (a) The design doesn't mention costUSD return. With plain text capture (no --output-format json/stream-json), cost will always be 0. This is acceptable for a subscription driver (no per-call cost) but should be stated explicitly in the design or a code comment.
- (b) The design doesn't specify `--model <model>` in the claude -p invocation. The reference driver always passes --model. Without it, claude uses its default model, ignoring the user's per-role config.
- (c) Non-zero exit → KindAuth is coarse (a rate limit or internal error would be misclassified as auth). Acceptable as a first implementation; the design acknowledges this. Finer classification can land later.

## Suggested ack reply
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