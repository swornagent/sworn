# Captain review — S04-inprocess-oai-driver
Date: 2026-07-06
Design commit: 4142edc70f2a31428d8a6cbd17c4a264649d1acd (design.md at f268da9)

## Pins

1. [escalate] §Key-design-decisions.D5 — Fail-fast preservation across the driver rewire is an untracked cross-slice assumption.
   What I observed: D5 maps `model.Error.Kind.String()` straight into `Result.ErrKind`, yielding
   `auth`/`credits`/`rate_limit`/`upstream`/`other`. Emission is contract-legal — `driver.go:124`
   documents `"credits"` as a sample `ErrKind`. The problem is downstream: the engine's terminal-halt
   (`internal/run/slice.go:490`) currently fires off the returned *error* via `model.IsTerminal(implErr)`,
   which halts on `KindAuth` AND `KindCredits`. That is the pre-rewire (direct-agent) path. After the S06
   rewire (still `planned`) the engine consumes `driver.Result`, and the subprocess siblings can only
   signal terminal through `Result.ErrKind` — which they deliberately collapse to `"auth"` for every
   terminal case (`subprocess.go:22-28`, the binding cross-driver contract). If S06's terminal set is
   `{auth}` only, an in-process `credits` exhaustion silently loses the halt that a subprocess `credits`
   failure (mapped to `auth`) still gets — a regression of the exact fail-fast the driver-contract-recut
   memory and spec R-03 protect.
   What to ask the implementer: decide and record what `Dispatch` returns as its *error* value. Two safe
   options: (a) return the underlying `*model.Error` so `model.IsTerminal` keeps firing regardless of how
   S06 reads `ErrKind`; and/or (b) track — as a Rule 2 item with acknowledgement — that S06 MUST treat
   `ErrKind ∈ {auth, credits}` as terminal to keep parity with the subprocess drivers. The assumption
   cannot ship as a silent inline choice.
   Citation: [[project_driver_contract_recut]], [[project_provider_error_taxonomy]]

2. [mechanical] §Key-design-decisions.D5/D7 — Reuse the shared `ErrKind*` constants; make the auth-preservation explicit.
   What I observed: `inprocess.go` lives in the SAME `driver` package as subprocess.go's exported
   `ErrKindConfig`/`ErrKindTransient`/`ErrKindAuth`/`ErrKindProvider`/`ErrKindProtocol`. D5/D7 hardcode the
   string literals `"config"`/`"protocol"`/`"transient"` and pass `me.Kind.String()` through raw.
   What to ask the implementer: reference the package constants for the shared values (a hardcoded literal
   is a typo-drift surface against the sibling drivers), and make `KindAuth → ErrKindAuth` an explicit
   mapping rather than an incidental `String()` collision, so the "reuse ErrKindAuth" contract is visible
   in the code, not implied. Folds into pin 1's mapping decision.
   Citation: [[project_driver_contract_recut]]

3. [escalate] §Key-design-decisions.D1 / AC-03 — Two registered identities, but the content-present guard is validated only for the chat/completions wire format.
   What I observed: D1 registers TWO drivers off one `InProcess` struct — `oai-inprocess` (backed by
   `model.OAI`) and `oai-responses-inprocess` (backed by `model.OpenAIResponses`). The S27 content-omitempty
   fix is present on the chat path (`oai.go:88`, `Content string json:"content"`) but the Responses path
   still carries `json:"content,omitempty"` (`openai_responses.go:103,135`) — and `internal/model` is out
   of scope to edit (AC-05). AC-03 is a universal SHALL, but the design's AC-03 test plan only exercises the
   chat path ("replaying the first turn's tool-only assistant message").
   What to ask the implementer: read `OpenAIResponses.Chat`'s request construction and confirm whether the
   Responses wire format can even hit the tool-only-content-drop (its input-item shape may make AC-03 moot
   for that identity). If it CAN drop content, AC-03 is unmet for `oai-responses-inprocess` and the fix
   would live in out-of-scope `internal/model` → this is a spec/scope tension for the Coach (narrow AC-03 to
   the chat identity, or `/replan-release`). Do not let the Responses identity ship an untested AC-03 guard.
   Citation: [[project_parallel_cold_start_broken]] (the "universal content-omitempty agent-loop bug")

4. [mechanical] §status.json — Rule 9 design-fit gate: no `design_decisions` field recorded.
   What I observed: `status.json` has no `design_decisions` array. The design labels only D5/D6 as "Type-2
   defaults"; D1 (the driver-identity model that S05's registry and S06's rewire consume) and D5 (the
   cross-driver error contract — see pin 1) are plausibly architecturally-significant → Type-1.
   What to ask the implementer: populate `status.json.design_decisions` with each decision classified
   Type-1/Type-2. Rule 9: an architecturally-significant choice classified Type-2, or a Type-1 with no
   recorded human decision, fails the design-fit gate closed. If D1/D5 are Type-1, the model may propose and
   classify but only the Coach records the decision.

5. [mechanical] §Key-design-decisions.D6 — Use the contract's documented `CostSource` value, not a new "nominal".
   What I observed: `driver.go:137` documents `CostSource` example `"estimated"`; `claude.go` uses
   `"provider-reported"`/`"unknown"`. D6 introduces `"nominal"`.
   What to ask the implementer: emit `CostSource: "estimated"` (the contract's own example) for the
   placeholder, or confirm `"nominal"` is a deliberately distinct value. The placeholder-vs-0 choice (Open
   Q2) is fine as a clearly-tagged estimate — it gives S08 an obvious value to replace and never fabricates
   a provider-reported number (honest-cost direction).
   Citation: [[project_telemetry_eval_foundation]]

6. [escalate] §Open-questions.Q1 — D5's narrowing of "structured-emission failure" is the correct reading; bless it explicitly.
   What I observed: AC-04 maps a structured-emission failure to `ErrKind=protocol`. D5 narrows this to keep a
   *classified* provider error (auth/credits/rate-limit) on the verdict call as its real `ErrKind` rather
   than folding it into `"protocol"`. The implementer flagged this as an interpretation, not a verbatim
   restatement.
   What to ask the implementer: this reading is correct and should be accepted — a transport auth/credits
   failure is not a protocol failure, and folding it into `"protocol"` would hide the very signal pin 1's
   fail-fast depends on. It needs one line of explicit Coach acknowledgement so the AC interpretation is
   on the record, not silently baked in.

## Summary
Pins: 6 total — 3 [mechanical], 0 [memory-cited], 3 [escalate]
Critical pins (if any): 1 (fail-fast preservation across the rewire) and 3 (Responses identity vs AC-03) —
either could ship the slice subtly broken (a lost terminal-halt, or an unmet universal AC on the second
identity) if unaddressed.

## Smaller flags (not pins, worth one-line acknowledgement)
- (a) Touchpoint `internal/driver/inprocess.go` is shared with S08 (`state: planned`). S08's spec lists it as
  a documented shared file and D6 acknowledges the region-split — no collision (sibling is not
  in_progress/implemented; serial track ownership resolves ordering).
- (b) S27 content-omitempty fix confirmed live at `oai.go:88` (not reverted by the release-wt base-sync).
- (c) Every cited symbol verified present against live code: `agent.Run` `(string, float64, []Message, error)`
  (`agent.go:81`), `agent.ErrMaxTurns` (:26), `defaultMaxTurns = 25` (:54), `agent.Agent.Chat` (:37),
  `model.NewClient` (`provider.go:87`, returns `Verifier` — the type-asserts are load-bearing),
  `model.StructuredOutput` (`client.go:52`), `OAI`/`OpenAIResponses.ChatStructured`,
  `driver.AssertWorktree` (`worktree.go:23`), and `driver.go:155`'s `Name()` doc example `"oai-inprocess"`
  (D1's name matches). The design is factually accurate — pins are about contract handoffs and scope, not
  wrong references.
- (d) Reviewed past a non-zero drift gate (track behind release-wt by 14 commits). Verified the residual
  drift is orthogonal TUI/log-view work touching neither `internal/driver/` nor `internal/model/`, and the
  track's `design.md` is the freshest copy (release-wt's is a 1-line stub). Documented as a Rule 2
  acknowledgement; the merge gate owns full-suite verification.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content verbatim-pasteable. -->

TL;DR Strong design — wrap-don't-rewrite, factually accurate against live code, correctly reuses the S01
contract and the ADR-0011 structured plumbing. 6 pins + 4 flags, and two of them (fail-fast handoff,
Responses/AC-03) need a Coach call before code:

1. **Fail-fast handoff (D5).** Emitting `credits`/`rate_limit`/`upstream` as `ErrKind` is contract-legal
   (`driver.go:124` lists `"credits"`), but the engine's terminal-halt keys off `model.IsTerminal` on the
   returned error today, and post-S06-rewire it will read `Result.ErrKind` — where the subprocess siblings
   collapse everything to `"auth"`. Decide what `Dispatch` returns as its error (prefer returning the
   underlying `*model.Error` so `IsTerminal` still fires), AND record a tracked Rule 2 note that S06 must
   treat `ErrKind ∈ {auth, credits}` as terminal. Don't let this ship as a silent assumption.
2. **Reuse the constants (D5/D7).** `inprocess.go` is in the same package as `ErrKindAuth`/`ErrKindConfig`/
   `ErrKindTransient`/`ErrKindProtocol` — use them, not string literals, and make `KindAuth → ErrKindAuth`
   explicit.
3. **Responses identity vs AC-03 (D1).** You register `oai-inprocess` AND `oai-responses-inprocess`. The
   S27 content-present fix is on the chat path (`oai.go:88`); the Responses type still has
   `content,omitempty` (`openai_responses.go:103`, out of scope to edit). Read `OpenAIResponses.Chat` and
   confirm the Responses wire format can't hit the tool-only-content-drop. If it can, AC-03 is unmet for
   that identity and the fix is out of scope — flag it back, don't paper over it.
4. **Rule 9 (status.json).** Populate `design_decisions` with Type-1/Type-2 classifications. D1 and D5 look
   Type-1 (they set contracts S05/S06 consume) — if so, that's a Coach-recorded decision, not a model one.
5. **CostSource (D6).** Use `"estimated"` (the `driver.go:137` example), not a new `"nominal"`. Placeholder
   estimate is fine — clearly tagged, honest, gives S08 an obvious replacement.
6. **D5 narrowing (Open Q1) — accepted.** Keeping a classified provider error as its real `ErrKind` on the
   verdict call (instead of folding to `"protocol"`) is the right reading; it protects the pin-1 signal.
   Proceed with it.

Flags (not pins): (a) inprocess.go touchpoint shared with S08 (planned) — acknowledged, no collision;
(b) S27 fix confirmed live at oai.go:88; (c) all cited symbols verified present; (d) reviewed past an
orthogonal 14-commit drift (TUI work, not driver/model) — design.md is the freshest copy.

§2 decisions: D1/D5 → classify as Type-1 and record (pin 4); D2/D3/D4/D7 acknowledged as sound Type-2.
§6 questions: Q1 accepted (pin 6); Q2 (placeholder cost) accepted as an `"estimated"`-tagged value (pin 5).

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Design is sound and accurate, but two pins need Coach judgement before code — the cross-driver fail-fast handoff to the unplanned S06 (pin 1, protects a binding contract) and the two-identity/AC-03 scope tension where the Responses fix would fall in out-of-scope internal/model (pin 3), plus potential Type-1 decisions (D1/D5) only the Coach can record.
-->
