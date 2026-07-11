# Design TL;DR — S04-inprocess-oai-driver

## User outcome (from spec.json)

An operator with OpenAI-compatible API keys keeps everything working through
ONE hardened in-process driver behind the same `driver.Driver` contract — the
agent loop and provider wire format become an implementation detail invisible
to the orchestrator, and the in-process verifier role runs the tool loop
before emitting its structured verdict (closes the sworn#55 gap for
in-process dispatch too).

## Approach

Wrap, don't rewrite. Two existing, already-correct mechanisms get one new
seam around them:

- `internal/agent.Run` — the multi-turn tool loop (25-turn cap as circuit
  breaker, workspace-confined executor, `cmd.Dir=root`). Untouched.
- `model.NewClient` + the OAI/OpenAIResponses clients' `Chat` /
  `ChatStructured` methods (ADR-0011 structured-output plumbing:
  `strictProjection`, `normaliseStructuredContent`). Untouched.

The new work is a single `driver.Driver` implementation (`InProcess`) in
`internal/driver/` that:

1. Resolves `DispatchInput.ModelID` to a concrete client via
   `model.NewClient` (already handles the full provider-prefix switch —
   openai, deepseek, groq, mistral, openrouter, cloudflare, github,
   openai-responses).
2. Type-asserts that client against `agent.Agent` (has `Chat`) to drive
   `agent.Run`, and against `model.StructuredOutput` (has `ChatStructured`)
   for the verifier's verdict emission. A client that fails either assertion
   is rejected by construction, not discovered mid-run.
3. For `Role=implementer`: runs `agent.Run` and maps its outcome to a
   `driver.Result`.
4. For `Role=verifier`: runs `agent.Run` for investigation, then makes
   exactly one `ChatStructured` call over the accumulated transcript against
   `DispatchInput.VerdictSchema` — the new mechanism that closes sworn#55 for
   in-process dispatch (today's `verify.RunAgentic` is zero-tool single-shot).

## Key design decisions

**D1 — One driver type, two registered identities.**
`model.NewClient` already does 100% of the provider-prefix branching
internally (openai → `*OAI`, openai-responses → `*OpenAIResponses`, etc.). Per
S05's rationale and S10's conformance spec, the registry expects two distinct
compiled-in drivers — "in-process oai" (chat/completions family) and
"in-process responses" (`/v1/responses`) — because they'll route from
different prefixes (sworn#31: `openai/` → Responses, `openai-completions/` →
legacy chat/completions). Rather than duplicate the Dispatch logic, `InProcess`
carries its own `Name() string` as an instance field set at construction
(`NewOAIChat(pcfg)` → `"oai-inprocess"`, `NewOAIResponses(pcfg)` →
`"oai-responses-inprocess"` — the first name matches the example already in
`driver.go`'s `Name()` doc comment). Both constructors produce the same
`InProcess` struct; `Dispatch` always re-resolves via `model.NewClient(in.ModelID, ...)`
so the two instances behave identically except for what they report as their
name. S05 (not this slice) decides which prefixes route to which registered
instance.

**D2 — Roles() declares Implementer + Verifier only.**
AC-01/AC-02 only specify those two roles; Captain dispatch is out of scope
here (no AC references it, no captain-shaped payload in this slice's spec).

**D3 — Token/duration metering without touching internal/agent.**
AC-01 requires `InputTokens`/`OutputTokens`/`DurationMS` populated, but
`agent.Run`'s signature (`(string, float64, []Message, error)`) doesn't
surface per-call `UsageBlock`, and editing `internal/agent` is explicitly
out of scope (AC-05; those edits belong to S08). Solution: an unexported
`chatMeter` type in `inprocess.go` implements `agent.Agent`'s `Chat` method
by delegating to the real client's `Chat` and accumulating
`resp.Usage.PromptTokens` / `CompletionTokens` across turns — pure
observation of a return value the driver already receives, zero change to
`internal/agent`. `chatMeter` is what gets passed into `agent.Run`, not the
raw client. `DurationMS` is `time.Since(start)` measured around the whole
`Dispatch` body.

**D4 — Verdict-transcript conversion is new, local, and narrow.**
`agent.Run` returns `[]agent.Message` (its own type, not `model.ChatMessage`).
The verifier path needs to replay that transcript into a `ChatStructured`
call, so `inprocess_verify.go` adds a small `toModelMessages([]agent.Message) []model.ChatMessage`
converter (role/content/tool-call-id/tool-calls passthrough, wrapping
`agent.ToolCall` into `model.ToolCall{Type: "function", ...}`). No existing
helper does this conversion (grepped the tree — `agent.Message` has no
consumer outside `internal/agent` today).

**D5 — Error classification (AC-04).**
A shared `classifyErr(err error) string`:
- `errors.Is(err, agent.ErrMaxTurns)` → `"transient"` (checked first, and
  wins regardless of any wrapped `*model.Error`, per AC-04's exact wording).
- else `model.AsError(err, &me)` → `me.Kind.String()` (`auth`, `credits`,
  `rate_limit`, `upstream`, `other`) — reuses the taxonomy already in
  `internal/model/errors.go`, no new enum.
- else `"other"`.

For the verifier's structured-emission step specifically, AC-04 requires a
`"protocol"` `ErrKind` on "structured-emission failure." Design choice: only
fall back to `"protocol"` when `classifyErr` would otherwise land on
`"other"` for that call (empty `Choices`, content that fails
`normaliseStructuredContent`'s non-empty/valid-JSON check, or an unclassified
error from `ChatStructured` itself). A *classified* provider failure (auth
revoked, credits exhausted, rate-limited) occurring on that same call keeps
its real `ErrKind` rather than being folded into `"protocol"` — so triage
still sees "your key was revoked" instead of a generic protocol bucket that
would send it down the wrong retry/escalation path. **Flagging this
narrowing of "structured-emission failure" for reviewer attention** (see
Open questions below) — I'm proceeding with it as the sensible reading, but
it's a interpretation of the AC text, not a verbatim restatement of it.

**D6 — CostUSD/CostSource is a placeholder in this slice.**
S08-honest-cost-telemetry (T4-resolution-loop) is the slice that wires
`Result.CostUSD` from the *confirmed* response model-id and true token split
through the unified pricing registry (its own AC text says exactly this, and
its touchpoints already list `internal/driver/inprocess.go` as a documented
shared file in the touchpoint matrix — this is the planned region-split, not
a collision). This slice computes a nominal placeholder identical in spirit
to `agent.computeCost`'s existing flat ~$2/1M-token estimate, tagged
`CostSource: "nominal"`, so the field is never fabricated as
provider-reported and S08 has an obvious value to replace. Real
`InputTokens`/`OutputTokens` (D3) are already honest and don't need S08 to be
useful themselves.

**D7 — Fail-closed guards.**
- Empty `WorktreeRoot` → `Status=error, ErrKind="config"` before calling
  `driver.AssertWorktree` (a caller-input problem, not a filesystem/git
  problem — kept distinct from `AssertWorktree`'s own error class).
- `driver.AssertWorktree(in.WorktreeRoot)` failure → `Status=error,
  ErrKind="config"`.
- Client fails the `agent.Agent` type-assertion → `Status=error,
  ErrKind="config"` (a model ID that resolves to a driver this wrapper
  cannot drive, e.g. a future `NewClient` addition without `Chat` — should
  never happen for the provider prefixes this driver is registered against,
  but fails closed instead of a nil-pointer panic).
- Verifier client additionally fails the `model.StructuredOutput`
  type-assertion → `Status=error, ErrKind="protocol"` (it can chat but
  cannot emit a verdict — same bucket as a structured-emission failure).
- `len(resp.Choices) == 0` after `ChatStructured` → checked explicitly,
  never indexed unchecked (no panics, per AC-04).

## Files touched (matches spec.json touchpoints exactly)

- `internal/driver/inprocess.go` — `InProcess` type, `NewOAIChat`/
  `NewOAIResponses` constructors, `chatMeter`, `classifyErr`, the
  implementer path.
- `internal/driver/inprocess_verify.go` — the verifier path: investigation
  loop + `toModelMessages` + the one `ChatStructured` verdict call.
- `internal/driver/inprocess_test.go` — httptest-server-backed tests (no
  paid dispatch), one per AC below.

No edits to `internal/agent` or `internal/model` (AC-05). No registry
registration (S05). No changes to `RunSlice`'s direct agent construction
(S06).

## Test plan → AC traceability

- **AC-01**: httptest server scripts a tool-call turn then a terminal turn
  for `Role=implementer`; assert `Status=ok`, `ResultText` equals the
  terminal turn's content, `InputTokens`/`OutputTokens` > 0,
  `DurationMS` > 0.
- **AC-02**: httptest server scripts an investigation turn (tool call +
  terminal text) then accepts a `ChatStructured`-shaped follow-up request;
  assert `Result.StructuredJSON` is the emitted object, unmodified and
  unvalidated by the driver (engine's job).
- **AC-03**: inspect the raw JSON body of the *second* turn's request (the
  one replaying the first turn's tool-only assistant message back to the
  model) and assert the assistant message's `"content"` key is present
  (`""`), never absent.
- **AC-04**: (a) a server that never stops requesting tool calls exhausts
  `cfg.MaxTurns` → assert `Status=error, ErrKind="transient"`; (b) a verifier
  server whose final call returns empty `choices` → assert `Status=error,
  ErrKind="protocol"`, no panic.
- **AC-05**: `go test ./internal/driver/...` passes; `git diff
  release-wt/2026-06-28-driver-contract..HEAD --stat` touches only the three
  files above (asserted by proof bundle, not a unit test).

## Open questions for the reviewer

1. **D5** narrows "structured-emission failure" (AC-04) to exclude
   classified provider errors (auth/credits/rate-limit/upstream) that happen
   to occur on the verdict call, keeping their real `ErrKind` instead of
   `"protocol"`. I think this is the right read (protocol failures and
   provider transport failures need different escalation handling), but it's
   an interpretation, not a literal restatement of the AC text — flagging
   before I build against it.
2. **D6** ships a nominal cost placeholder rather than blocking on S08. If
   the reviewer would rather this slice do nothing with `CostUSD` (leave it
   `0`) instead of a placeholder estimate, that's a one-line change — noting
   the choice now so it isn't silently baked in.

Both are Type-2 (narrow, local, reversible) defaults — proceeding unless the
Captain pushes back.
