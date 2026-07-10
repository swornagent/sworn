# Design TL;DR — S06-loop-dispatch-rewire

## User outcome (from spec.json)

RunSlice performs its captain, implement, and verify legs exclusively through
registry-resolved `Driver.Dispatch` — no factory fields, no provider wire
types in the orchestration path, verdicts engine-validated from
`Result.StructuredJSON` — so the nil-factory SIGSEGV class is unrepresentable
and a toolless dispatch cannot reach a loop role.

## Approach

Every model dispatch RunSlice makes today goes through
`opts.NewAgent(modelID)` returning a raw `agent.Agent`, which the leg
functions consume via wire types (`model.ChatMessage`, `ChatResponse`). This
slice replaces that with ONE seam: `registry.Resolve(modelID, role)` →
`driver.Dispatch(DispatchInput)` → `driver.Result`. Prompt assembly stays
orchestrator-side (`SystemPrompt`/`Payload` in `DispatchInput`); loop
execution, wire formats, and token/cost metering live behind the driver
(already built and verified in S02–S05).

Leg-by-leg mapping (grounded against live code in this worktree):

| Leg | Today | After this slice |
|---|---|---|
| Design TL;DR (slice.go:292) | `opts.NewAgent(firstModel)` → `design.Generate(…, agent)` → tool-less `Chat` inside `internal/design` | `Resolve(firstModel, RoleCaptain)` → `design.Generate(…, d, firstModel, worktreeRoot, timeout)` → `d.Dispatch(Role=captain)`; §-header validation + design.md write stay in `internal/design` |
| Captain review (slice.go:340,366) | `opts.NewAgent(firstModel)` → `captain.Review(…, agent, …)` → tool-less `Chat` inside `internal/captain` | `Resolve(firstModel, RoleCaptain)` → `captain.Review(…, d, firstModel, …, timeout)` → `d.Dispatch(Role=captain)`; pin parsing + review.md write stay in `internal/captain`; `ReviewResult` carries the leg's `driver.Result` for telemetry |
| DoR gate (implement.go:54-70, ready.go:25) | `agentVerifier{agent.Agent}` adapts `Chat` to `reqverify.Verifier` | `driverVerifier{d, modelID, worktreeRoot}` adapts `d.Dispatch(Role=captain)` to the same `reqverify.Verifier` interface (already wire-free: `Verify(ctx, system, payload) (string, float64, int64, int64, error)`) |
| Implement (slice.go:458,473) | `opts.NewAgent(implModel)` → `implement.Run(…, agent)` → `agent.Run` tool loop | `Resolve(implModel, RoleImplementer)` → `implement.Run(ctx, workspaceRoot, specPath, priorFeedback, d, implModel, timeout)` → `d.Dispatch(Role=implementer)`; returns `(driver.Result, error)` |
| Verify (slice.go:696,712) | `opts.NewAgent(verifierModel)` → `verify.RunAgentic(…, agent)` → type-assert `model.StructuredOutput`, one `ChatStructured` call | `Resolve(verifierModel, RoleVerifier)` → `verify.RunAgentic(ctx, AgenticInput{…}, d)` → `d.Dispatch(Role=verifier, VerdictSchema=verifierEmitSchema)`; engine validates `Result.StructuredJSON` via the retained `acceptStructuredVerdict` + `baton.ValidateSchema` path |
| First-pass (slice.go:643) | `verify.RunFirstPass` — deterministic, $0 | **Untouched, byte-identical** (AC-05) |

**Fail-fast resolution before any dispatch (AC-02).** RunSlice resolves all
role legs up front, immediately after option validation: `VerifierModel` for
`RoleVerifier`, every entry of the escalation list for `RoleImplementer`, and
the first model for `RoleCaptain`. Implementer/verifier resolution failure
returns the registry's descriptive error (it already names model, role, and
registered alternatives — S05 AC-02/AC-03 vocabulary) BEFORE any model
dispatch. No code path ever holds a nil or unresolved driver — the SIGSEGV
class (slice.go:193-201's eval-supervisor nil-default patch) is deleted along
with the fields it patched.

## Key design decisions

**D1 — Registry injection: `RunSliceOptions.Registry *registry.Registry`,
nil → `registry.Default(model.ProviderConfigFromEnv())`.**
The two factory fields are deleted (not deprecated) from `RunSliceOptions`
AND from `run.Options` (run.go:65-71 only exist to be passed through at
run.go:224-225). `newAgentFromModel`/`newVerifierFromModel` (run.go:343-369)
are deleted — their `CapChat` capability gate is subsumed by the registry's
role check, which is the S05 design intent ("capability IS the role set").
Tests inject fake drivers through a test registry (`registry.New()` +
`Register(Entry{Driver: fakeDriver, Prefixes: […]})`), never through
factories — exactly AC-01's injection contract. The cmd call sites
(cmd/sworn/run.go:135, cmd/sworn/task.go:200) never set the factory fields
(that was the SIGSEGV bug), so they compile unchanged and inherit the
default registry.

**D2 — RoleCaptain: declared by the in-process drivers only; captain-leg
resolution failure routes to the existing design-gate deferral, not a hard
halt. [FLAG for Captain review]**
No driver declares `RoleCaptain` today (S04 design D2 deliberately deferred
it: no AC described it — this slice's AC-01 now does). The captain-family
dispatches (design TL;DR, captain review, DoR check) are single **tool-less**
judgement calls; dispatching them through the tool-loop path would hand the
reviewer file-edit tools, violating the review's read-only nature. So:

- `InProcess.Roles()` gains `RoleCaptain`; a new `dispatchCaptain` path in
  `internal/driver/inprocess` makes exactly one tool-less `meter.Chat` call
  (system + payload, nil tools) and returns `StatusOK` + `ResultText` +
  economics. Error classification reuses `classifyErr` minus the max-turns
  arm (no loop).
- The subprocess drivers (claude-cli, codex) keep captain **undeclared**
  this slice: `claude -p` is an edit-capable loop we cannot make read-only,
  and no AC requires subprocess captain dispatch. When the first escalation
  model is a subprocess prefix, `Resolve(model, RoleCaptain)` fails and the
  leg routes through the EXISTING `recordDesignGateDeferral` fail-open path
  (slice.go:141 — durable Rule 2 record, tracked sworn#51), with the
  registry's descriptive role error embedded in the deferral reason. This
  preserves today's semantics class: a captain gate that cannot run defers
  loudly and the run proceeds; it does not brick claude-cli-driven runs.
- AC-02's hard-error contract applies to the implement and verify legs
  (where an unserved dispatch would be a wrong-answer path); the captain leg
  is a gate with an established fail-open-with-deferral contract. This
  reading is flagged for the Captain because AC-02 says "any role leg".

**D3 — R-03 answer: ONE terminal-kind predicate in the contract package,
consumed at both points. Set = {auth, credits}, exactly.**
`internal/driver/driver.go` gains:

```go
// ErrKindCredits — promoted from inprocess's private errKindCredits so the
// terminal vocabulary has a single source (S04 Coach ack binding).
const ErrKindCredits = "credits"

// TerminalErrKind reports whether kind can never succeed on retry or model
// escalation (revoked/missing credentials, exhausted credits). The set is
// {auth, credits} — the S04 Coach acknowledgement (T3 captain-proceed.md,
// 2026-07-10) is the binding record: subprocess drivers collapse all
// terminal cases to auth; the in-process driver emits credits as its own
// kind. An auth-only check silently loses the credits fail-fast.
func TerminalErrKind(kind string) bool {
    return kind == ErrKindAuth || kind == ErrKindCredits
}
```

`inprocess`'s private `errKindCredits` is replaced by the contract constant.
The two consumption points both call `driver.TerminalErrKind` — no second
hand-synced set:

1. **Implement leg** (replaces `model.IsTerminal(implErr)` at slice.go:487):
   after `implement.Run` returns `(implRes, implErr)`, if
   `driver.TerminalErrKind(implRes.ErrKind)` RunSlice returns the BLOCKED
   sentinel immediately (`errVerdictBlockedPrefix` + "terminal driver error
   (auth|credits): … — halting; check provider credentials"), before the
   triage path — routing to /replan-release, never retry/escalate, exactly
   the S09 AC1 property being preserved across the transport swap.
2. **Verify leg** (AC-03's "terminal error kinds -> BLOCKED"): in the re-cut
   `RunAgentic`, a dispatch whose `Result.ErrKind` satisfies
   `driver.TerminalErrKind` maps to BLOCKED with gate
   `verifier_terminal_error` and the "halting; check verifier provider
   credentials" tail — the `blockedTerminal` semantics with the kind read
   from `Result.ErrKind` instead of `model.AsError`.

Tests assert BOTH `ErrKind="auth"` AND `ErrKind="credits"` halt immediately
at BOTH points (four cases), plus a non-terminal kind (`transient`) entering
the normal triage path — the R-03 regression net.

**D4 — Verify transport swap: `RunAgentic(ctx, AgenticInput, d)` with the
acceptance path preserved verbatim (R-01).**

```go
type AgenticInput struct {
    Spec, Diff, Proof string
    ModelID           string
    WorktreeRoot      string
    Timeout           time.Duration
}
func RunAgentic(ctx context.Context, in AgenticInput, d driver.Driver) (verdict.Result, error)
```

Inside: build `DispatchInput{Role: RoleVerifier, ModelID, SystemPrompt:
verifierRolePrompt, Payload: buildPayload(spec, diff, proof), VerdictSchema:
verifierEmitSchema, WorktreeRoot, Timeout}`; dispatch; then map:

| Dispatch outcome | Verdict (unchanged semantics) |
|---|---|
| `driver.TerminalErrKind(res.ErrKind)` | BLOCKED, gate `verifier_terminal_error` (D3 point 2) |
| any other dispatch error / `StatusError` | INCONCLUSIVE, gate `verifier_structured_dispatch` (fail-closed; cost fields still recorded from `res`) |
| `StatusOK` with empty `Result.StructuredJSON` | INCONCLUSIVE, gate `verifier_structured_dispatch` ("missing structured output" — the old empty-choices class) |
| `StatusOK` with `StructuredJSON` | `acceptStructuredVerdict(string(res.StructuredJSON), res)` |

`acceptStructuredVerdict` keeps its body verbatim — same stamping
(`schema_version`, `$schema`), same `baton.ValidateSchema("verifier-verdict-v1", …)`
call, same malformed→INCONCLUSIVE / invalid→INCONCLUSIVE / valid→mapped
`verdict.Result` arms — only its cost/usage source changes: signature becomes
`acceptStructuredVerdict(emitted string, res driver.Result) verdict.Result`,
populating `CostUSD`/`InputTokens`/`OutputTokens`/`DurationMS`/`ModelIDConfirmed`
from the `Result` economics fields instead of `*model.UsageBlock` +
`computeAgenticCost` (AC-05 plumbing pin; honest values land in S08). The
`verifier_structured_unsupported` INCONCLUSIVE arm (agent lacking
`model.StructuredOutput`) becomes unrepresentable at this layer — the
in-process driver already fails that closed pre-dispatch as
`ErrKindProtocol` (inprocess_verify.go:33-37), which maps to INCONCLUSIVE
via the dispatch-error arm; its test adapts to that path rather than being
deleted. Existing verify_test.go acceptance/validation tests are **adapted
(fed `driver.Result` instead of fake `ChatResponse`), not rewritten** —
any behaviour change must fail one of them first (R-01 mitigation).
`verify.Input`/`RunFirstPass` and the whole deterministic first-pass are
untouched. Standalone `sworn verify --agentic` (cmd/sworn/verify.go:111-126)
moves onto the same seam: `registry.Default(cfg).Resolve(model,
RoleVerifier)` → `RunAgentic` with `WorktreeRoot` = the resolved repo root.

**D5 — Implement seam: prompts orchestrator-side, loop driver-side.**

```go
func Run(ctx context.Context, workspaceRoot, specPath, priorFeedback string,
    d driver.Driver, modelID string, timeout time.Duration) (driver.Result, error)
```

`implement.Run` keeps everything it owns today (state gate + DoR, spec/proof
record generation, `implemented` transition) and keeps assembling
`prompt.Implementer()` + the user prompt (same strings, including the
priorFeedback truncation) — but hands them to
`d.Dispatch(DispatchInput{Role: RoleImplementer, ModelID, SystemPrompt,
Payload, WorktreeRoot: workspaceRoot, Timeout: timeout})` instead of
`agent.Run`. It returns the `driver.Result` so slice.go's `appendDispatch`
sources cost/duration/tokens/model-id from Result fields (AC-05).

Max-turns detection: the in-process driver preserves the error chain
(`agent.ErrMaxTurns` stays wrapped — S04 Coach ack pin 1), so slice.go's
`errors.Is(implErr, agent.ErrMaxTurns)` PAGE path keeps firing unchanged.
`internal/run` retains its `internal/agent` import for that sentinel only —
a sentinel error, not a wire type; AC-04's boundary is the four named wire
types (same distinction internal/scheduler already relies on with
`agent.MaxTurnsSentinel`).

Timeout plumbing: slice.go currently wraps the leg contexts with
`implementTimeout`; that wrap stays (behaviour preserved), AND
`DispatchInput.Timeout` is set to the same value explicitly — necessary
because the in-process driver's zero-Timeout default is 300s
(inprocess.go:48), which would silently cap a 15-minute implement leg.

**D6 — R-04 answer: one shared proxy predicate, extracted from
`model.FromEnv`; in-process drivers' client resolution goes through it.**
Today `InProcess.newClient` defaults to `model.NewClient` (inprocess.go:80,86)
— proxy-blind — while `registry.proxyRouting()` (registry.go:381-392)
re-implements FromEnv's login condition to ADVERTISE ViaProxy. After the
rewire that would mean: capabilities claims proxy, dispatch goes direct —
the exact S06b/OpenRouter keyless-credits regression R-04 names. Fix:

1. **Extract the predicate** into `internal/model`:
   `func ProxyRoute(modelID string) (baseURL, token string, ok bool)` —
   `SWORN_DIRECT` check, `account.Load(filepath.Dir(account.CredentialsPath()))`,
   `account.IsLoggedIn`, `account.Endpoint(creds, modelID)` — the exact
   block at config.go:67-94, minus client construction.
2. **`model.FromEnv` refactors onto it** (behaviour identical — the proxy
   arm constructs the same `OAI`/`OpenAIResponses` values from the returned
   URL + token).
3. **New `model.ResolveLoopClient(modelID string, pcfg ProviderConfig)
   (Verifier, error)`** — the FromEnv-equivalent resolution the in-process
   drivers use as their `newClient` default (replacing bare `NewClient`):
   proxy route when `ProxyRoute` says so (openai/openai-responses →
   `OpenAIResponses`, else `OAI`, same as FromEnv's arm), otherwise
   `NewClient(modelID, pcfg)` direct.
4. **`registry.proxyRouting()` delegates** to `model.ProxyRoute(prefix +
   "/probe")` — enumeration and dispatch now evaluate literally the same
   function; the hand-synced duplicate is deleted. No second table.

Reachability test (R-04's binding): set `XDG_CONFIG_HOME` to a temp dir
containing a logged-in `sworn/credentials.json`, `SWORN_PROXY_URL` to an
httptest server (the documented test-only override, Coach ack pin B);
assert (a) `registry.Default(cfg).Drivers()` advertises `openai/` ViaProxy,
(b) a `Resolve("openai/gpt-x", RoleImplementer)` → `Dispatch` actually hits
the httptest proxy host (request observed server-side), and (c) with
`SWORN_DIRECT=1` BOTH the advertisement and the dispatch route flip off
together — same predicate, observed from both surfaces.

**D7 — Registry config source: `model.ProviderConfigFromEnv()`, widened
with SWORN_* fallbacks. [FLAG for Captain review]**
The default registry must be built from the same config `sworn capabilities`
uses (`ProviderConfigFromEnv()`, capabilities.go:32) or the R-04 predicate
splits on the key half. But the loop today reads the SWORN_* namespace via
`FromEnv`, and `ProviderConfigFromEnv` only aliases OPENAI/GOOGLE — so a
user with only `SWORN_OPENROUTER_API_KEY` set (the documented worker setup)
would lose direct dispatch. Fix: extend `ProviderConfigFromEnv` so every
provider key uses `envOrAlias(CANONICAL, SWORN_CANONICAL)` — canonical wins,
SWORN_* is fallback, per the existing envOrAlias contract. This also makes
`sworn capabilities` truthful for SWORN_*-only environments. Small,
backward-compatible widening; flagged because it touches behaviour outside
the literal touchpoints.

**D8 — AC-04 import-boundary test: AST selector scan, not ImportsOnly.**
`internal/run/imports_test.go` `TestNoWireImports` covers `internal/run`,
`internal/verify`, `internal/scheduler` (relative dirs `.`, `../verify`,
`../scheduler`), parsing every `.go` file **including `_test.go`** fully and
failing — naming package, file, and identifier — on any selector
`<modelAlias>.{ChatMessage|ToolDef|ChatResponse|ToolCall}` where the alias
binds to an `internal/model` import. A plain import ban (the
`internal/driver` pattern) is wrong here: `verify.Input.Verifier` and
post-rewire `internal/run` still legitimately reference non-wire `model`
identifiers (`model.Verifier` interface, `ProviderConfigFromEnv`), and
AC-04's text names the four wire types, not the package.

**D9 — Deletions (grep-verified in this worktree).**
- `RunSliceOptions.NewAgent` / `.NewVerifier` + the nil-default patch
  (slice.go:57-63, 193-201) — AC-01's core deletion.
- `run.Options.NewAgent` / `.NewVerifier` + defaults (run.go:65-71,107-111)
  and the passthrough (run.go:224-225).
- `newAgentFromModel` / `newVerifierFromModel` (run.go:343-369).
- `RunSliceOptions.InterpretVerifier` (slice.go:79-83) — dead since the
  ADR-0011 keystone deleted the stateless interpreter; only the declaration
  remains (grep: zero readers). [FLAG — deletion of a dead public field,
  strictly outside the spec's in_scope list but inside "no provider wire
  types in the orchestration path".]
- `captureVerifier`/`extractViolations` are already gone (slice.go:883-911
  notes); nothing further there.

**D10 — Telemetry plumbing (AC-05).**
Every `appendDispatch` call site switches source to the leg's
`driver.Result`: implement leg from the returned Result
(`CostUSD`/`DurationMS`/`InputTokens`/`OutputTokens`/`ModelID` →
`ModelIDConfirmed`); captain leg from `ReviewResult.Dispatch driver.Result`
(new field; replaces the slice.go-measured duration and the usage-derived
cost); verifier leg unchanged in shape — `verdict.Result`'s
DurationMS/tokens/ModelIDConfirmed fields are now populated by
`acceptStructuredVerdict` from the Result (D4). `first_pass` dispatch record
stays `Model: "deterministic", CostUSD: 0`. The captain record's
only-when-pins-exist quirk (slice.go:390-399) is preserved verbatim —
changing when records are written is S08's honesty scope, not this slice's
plumbing scope. Honest population semantics (real pricing, provider-reported
cost) land in S08; this slice pins the SOURCE only.

## Files to touch

Spec touchpoints:
- `internal/run/slice.go` — options re-cut, upfront resolution, leg rewires, R-03 point 1, telemetry
- `internal/run/slice_test.go` — fake-driver test registry; adapted cases
- `internal/run/imports_test.go` — NEW: TestNoWireImports (D8)
- `internal/verify/verify.go` — RunAgentic re-cut + acceptStructuredVerdict source swap (D4); RunFirstPass untouched
- `internal/verify/verify_test.go` — adapted acceptance tests + terminal-kind cases
- `internal/implement/implement.go` — Run signature + dispatch seam (D5)
- `internal/implement/implement_test.go` — fake-driver adaptation

Required beyond the literal touchpoints (each named above; the track merges
as one unit so none of this reaches the integration branch mid-flight):
- `internal/driver/driver.go` — `ErrKindCredits`, `TerminalErrKind` (D3)
- `internal/driver/inprocess/inprocess.go` (+`inprocess_test.go`) — RoleCaptain + `dispatchCaptain` (D2); `newClient` default → `ResolveLoopClient` (D6); `errKindCredits` → contract const
- `internal/driver/registry/registry.go` (+`registry_test.go`) — `proxyRouting` delegates to `model.ProxyRoute` (D6)
- `internal/model/config.go` — extract `ProxyRoute` + `ResolveLoopClient`; `FromEnv` refactored onto them (D6)
- `internal/model/provider.go` — `ProviderConfigFromEnv` SWORN_* fallbacks (D7)
- `internal/run/run.go` — Options field deletion + Registry passthrough (D9/D1)
- `internal/captain/review.go` (+test) — dispatch via driver; `ReviewResult.Dispatch` (D10)
- `internal/design/tldr.go` (+test) — dispatch via driver
- `internal/implement/ready.go` — `agentVerifier` → `driverVerifier` (D5/D2)
- `cmd/sworn/verify.go` — agentic path resolves via registry (D4)
- run-package test files currently injecting factories (`run_test.go`,
  `slice_terminal_test.go`, `capabilities_test.go`, `factory_default_test.go`,
  `cold_start_test.go`, `dispatch_quadrant_test.go`) — adapted to test
  registries; `factory_default_test.go` becomes the nil-Registry-default
  test; `capabilities_test.go`'s newAgentFromModel cases are superseded by
  registry role-check tests
- cmd/sworn/run.go, cmd/sworn/task.go — compile unchanged (never set the
  deleted fields); verified, not assumed

## Acceptance-criteria traceability

- **AC-01** → D1 (fields deleted, registry injected, test-registry
  injection), D2 (captain leg), D5 (implement leg), D4 (verify leg).
  Tests: `TestRunSliceDispatchesAllLegsViaRegistry` (fake drivers record
  which roles were dispatched), plus adapted slice_test.go suite.
- **AC-02** → upfront resolution block. Test: `TestRunSliceResolutionFailure`
  (unknown prefix; role-incapable driver) asserts the error names model,
  role, and registered alternatives AND that zero dispatches were made
  (fake driver records calls).
- **AC-03** → D4 table + D3 point 2. Tests: adapted verify acceptance suite
  (valid PASS/FAIL/BLOCKED verdicts; malformed JSON → INCONCLUSIVE;
  schema-invalid → INCONCLUSIVE; missing StructuredJSON → INCONCLUSIVE)
  plus `ErrKind=auth` and `ErrKind=credits` → BLOCKED.
- **AC-04** → D8. Test: `TestNoWireImports` over the three packages, failing
  with offending package + import named.
- **AC-05** → D10 (Result-sourced telemetry), first-pass untouched (its
  existing tests unchanged verify this), and the slice test commands:
  `go test ./internal/run/... ./internal/verify/... ./internal/implement/...`.

## Spec risks — how the design answers them

- **R-01 (verdict-semantics drift):** `acceptStructuredVerdict` body and
  `baton.ValidateSchema` call are retained verbatim; only the economics
  source changes. Existing verify_test.go tests adapted, not rewritten; the
  D4 table is the complete behaviour map and each row has a test.
- **R-02 (cross-package fixture regression):** full
  `go test -timeout 300s ./...` runs before any state transition to
  `implemented` (the S05-strict-reader lesson — board.json fixtures broke in
  `internal/board` + `cmd/sworn`; named in journal.md). Highest-risk
  packages here: `cmd/sworn` (verify.go, run.go, task.go compile against
  the re-cut signatures) and `internal/scheduler` (worker fakes).
- **R-03 (terminal ErrKind set):** D3 — `driver.TerminalErrKind`, set
  exactly {auth, credits} per the S04 Coach ack (T3 captain-proceed.md,
  2026-07-10), ONE predicate at BOTH consumption points, four halt tests
  (auth/credits × implement/verify) plus a transient-continues test.
- **R-04 (advertised-vs-actual routing):** D6 + D7 — `model.ProxyRoute` is
  the single predicate behind FromEnv, `ResolveLoopClient`, and the
  registry's ViaProxy/keyProbe; in-process drivers default to
  `ResolveLoopClient`; the three-part reachability test observes the proxy
  route server-side under the login condition and observes both surfaces
  flip together under `SWORN_DIRECT=1`.

## Test plan (slice-relevant commands)

```
go test ./internal/run/... ./internal/verify/... ./internal/implement/...   # spec AC-05
go test ./internal/driver/... ./internal/model/... ./internal/captain/... ./internal/design/...
go test -timeout 300s ./...                                                  # R-02 gate, before `implemented`
gofmt -l / go vet on every changed package                                   # newline-corruption hazard sweep
```

## Out of scope (unchanged from spec)

- The parallel/scheduler startup sweep (S07) — the scheduler's
  `RunSliceFn` seam is untouched; only its package joins the AC-04 scan.
- Telemetry honesty (S08) — this slice pins Result as the plumbing source
  only; nominal costs remain nominal and CostSource remains "estimated".
- `RunFirstPass` — byte-identical, tests unchanged.

## Design-level risks / pins for the reviewer

1. **D2's AC-02 reading** (captain resolution failure = deferral, not hard
   halt) is the one place the design interprets rather than transcribes the
   spec. The alternative — hard-fail the run when the implementer model's
   prefix cannot serve captain — bricks every claude-cli/codex run at the
   design gate. Needs an explicit PROCEED/override.
2. **D7's config widening** changes which env vars light up direct dispatch
   (strictly additive: canonical wins, SWORN_* becomes a fallback
   everywhere). Flagged because it is behaviour outside the touchpoints.
3. **D9's InterpretVerifier deletion** — dead field, but it is a public
   struct field; if anything external (private harness) sets it, deletion is
   a compile break there. Repo-internal grep shows zero writers.
4. Signature changes to `implement.Run`, `verify.RunAgentic`,
   `captain.Review`, `design.Generate` are breaking within the module —
   all callers are enumerated in "Files to touch"; no exported-API
   compatibility promise exists (internal packages + cmd).
