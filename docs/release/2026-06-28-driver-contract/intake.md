---
title: 'Release intake — 2026-06-28-driver-contract'
description: 'Re-seam sworn so the orchestrator drives every agentic dispatch through a stable Driver contract (one orchestrator, N drivers). Default driver delegates the loop to a real agent CLI (claude-cli), fixing sworn#35 and unblocking subscription-based loop runs. Re-cut 2026-07-02 in canonical form (board.json + spec-v1 + EARS).'
---

# Release Intake: `2026-06-28-driver-contract`

> **Re-cut note (2026-07-02).** This release was originally planned 2026-06-28 in
> pre-cutover form (`index.md` frontmatter board + 9 `spec.md` slices, placeholder
> `N-DRV` needs, no `intake.md`, no `board.json`). No implementation ever started:
> zero branches/worktrees, all 9 slices `planned` (verified live 2026-07-02). This
> intake is a fresh planning pass that treats the 2026-06-28 artefacts as raw
> material and re-cuts the plan in canonical form. Starter context:
> `docs/captures/2026-07-02-driver-contract-replan-starter.md`.

## Release goal

Re-seam sworn so the orchestrator drives every agentic dispatch through a stable
**Driver contract** — "one orchestrator, N drivers." The orchestrator never
constructs a provider wire message and never owns the agent tool loop; both live
behind `Driver.Dispatch`. The default driver is a **subprocess agent driver**
that delegates the loop to a real agent CLI (claude-cli first), which is the fix
for sworn#35 (claude-cli/anthropic advertise Chat but ignore tools; `cliDriver.Chat`
sets no `cmd.Dir`) — the confirmed blocker for running `sworn loop` on a Claude
subscription. The existing hardened in-process OAI agent loop becomes ONE driver
behind the same contract, an option rather than the default. "Shipped" looks
like: `sworn loop --release 2026-07-01-loop-cli-ux` runs end-to-end with
implementer sonnet / verifier opus dispatched through the subprocess driver (the
queued dogfood this release unblocks).

## Source of truth

- **Human stakeholder**: Brad (project owner)
- **Tracking issue / epic**: consumes sworn#35, #15, #31, #19, #55, #70 as planning input (see "What's currently broken")
- **Related captures**:
  - `docs/captures/2026-07-02-driver-contract-replan-starter.md` (this re-cut's commission)
  - `docs/captures/2026-06-28-sworn-architecture-recommendation.md` (the keystone recommendation §1–§3; Driver contract = recommendation #4)
  - `docs/captures/2026-06-28-sworn-eval-findings.md` (the three-model dogfood that proved the in-process loop DOA)
  - `docs/captures/2026-06-28-bash-coachloop-learnings.md` (the reference runtime-driver contract this ports; coach-loop now retired)
  - `docs/captures/2026-06-27-surface-seam.md` ("three surfaces, one core" — this release adds the dual "one orchestrator, N drivers")
- **Related memory entries**: `project_sworn_operational_loop_pivot` (sworn IS the loop now), `project_model_layer_service_refactor` (sparse provider×capability matrix — the thing S04 replaces), `project_loop_verifier_fidelity` (loop verifier goes agentic, stateless judge removed), `project_keystone_structured_outputs` (verifier-verdict-v1 landed — the seam moved since the 2026-06-28 cut), `project_parallel_cold_start_broken` (nil-factory SIGSEGVs etc.)

## What moved since the 2026-06-28 cut (why re-ground, not resume)

- The **verifier-verdict-v1 keystone** landed inside the loop (schema-constrained
  verifier output via `ChatStructured`; prose scrapers deleted) — the dispatch
  seam T2 (S05/S06) was scoped against no longer matches live code.
- **PR #78** (11 conformance fixes) and the **render-drift release** (board.json
  oracle for loop/MCP/TUI/CLI, fail-closed drift guard) merged to `release/v0.1.0`.
- **Baton v0.7.0** shipped; re-vendor tracked as sworn#48 (not this release).
- The **coach-loop reference is retired** and schema-incompatible with baton
  v0.7.0 — S08-differential-validation's reference implementation no longer
  exists live (archive at `~/projects/fired/baton-backup`).

## Users and their gestures

- **Operator on a Claude subscription running `sworn loop`**: implementer and
  verifier roles dispatch through the claude-cli subprocess driver with tools
  actually honoured and the child process running in the slice's worktree; the
  loop reaches `verified` without an API key. Before: sworn#35 — the dispatch is
  a toolless one-shot in the wrong directory, so the run was stopped pre-spend.
- **Operator with OpenAI-compatible API keys**: existing models keep working,
  now through the in-process oai driver behind the same contract — no behaviour
  regression, one hardened loop instead of a provider×capability matrix.
- **Engine developer adding a model/provider**: registers a driver; resolution
  answers "is a driver registered for this model + capability" fail-fast at
  startup (replaces the sparse capability matrix; sworn#15 territory).
- **Operator reading loop telemetry**: every dispatch records duration, token
  split, real cost, and the confirmed model-id (sworn#70); a subscription
  dispatch whose CLI reports cost 0 is recorded honestly as such, not as $0.00
  API spend.

## What's currently broken or missing

- **sworn#35** — claude-cli/anthropic advertise `Chat` but ignore tools;
  `cliDriver.Chat` sets no `cmd.Dir`. Confirmed blocker: the 2026-07-02 attempt
  to run `sworn loop` with implementer sonnet / verifier opus via claude-cli was
  stopped pre-spend on exactly this.
- **sworn#55** — the engine's "agentic" verifier is a single-shot `ChatStructured`
  call with no tool loop: it cannot re-run tests or read live repo state (Rule 7
  gap; the structured-output half of the 2026-06-27 decision landed, the
  tool-loop half did not).
- **sworn#15** — adding a provider means editing the central `NewClient()` switch
  in `internal/model/provider.go`; touchpoint-collision bottleneck (Type-1
  classified in the issue).
- **sworn#31** — `openai/` prefix routes to legacy chat/completions; Responses-only
  models fail. Rename `openai/` → responses driver, `openai-completions/` → legacy.
- **sworn#19** — codex exec subprocess driver: an open Rule-2 deferral from S63
  sitting in `internal/model/cli.go`.
- **sworn#70** — implementer + agentic-verifier cost telemetry is nominal flat
  $2/1M; the pricing registry (`PriceForModel`) is dark code with zero call
  sites; Anthropic's correctly computed `CostUSD` is discarded by the agent loop.
- **Root cause (architecture capture §2)**: sworn reimplemented the agent loop
  and every provider's wire format in-process; one struct tag
  (`content,omitempty`) took down every provider at once; the three-model
  dogfood reached `verified` for zero slices.

## What the human wants

(Draft needs register — to be confirmed/amended during discovery before specs
are cut. Trace gate binds every N-NN to at least one slice's `covers_needs`.)

- **N-01**: A single `Driver` contract at the process boundary — an agentic
  dispatch is `Driver.Dispatch(...) -> Result`; no provider wire type and no
  tool-loop logic is visible to the orchestrator (run/scheduler/state packages).
- **N-02**: A subprocess agent driver (claude-cli first) that delegates the
  whole agentic loop to the agent CLI, honours tools by construction, runs in
  the slice worktree (`cmd.Dir` set, target-asserted per Rule 11), and returns a
  normalized `Result` — so `sworn loop` works on a Claude subscription.
- **N-03**: The existing in-process OAI agent loop available as one driver
  behind the contract (an option, not the default), with no behaviour
  regression for OpenAI-compatible API-key users.
- **N-04**: Model→driver resolution with fail-fast capability checking at
  startup ("no driver for model X" / "driver lacks capability Y"), replacing
  the sparse provider×capability matrix.
- **N-05**: `RunSlice` and the parallel scheduler dispatch implement/verify
  exclusively through resolved Drivers — the nil-factory class is gone by
  construction.
- **N-06**: Every dispatch records duration, token split, real cost (honest
  zero/unknown for subscription CLIs), and confirmed model-id (sworn#70).
- **N-07**: A behavioural conformance suite every registered driver must pass,
  so a new driver is provably contract-correct before it can dispatch.
- **N-08**: **Role-universality (decided 2026-07-02)**: any registered driver
  can serve any loop role it declares capability for — the verifier role
  dispatches through the Driver contract like every other role, and where the
  driver provides a real tool loop (subprocess CLI re-running tests in the
  worktree) that closes sworn#55. Capability is declared per-role by the
  driver, checked fail-fast at resolution; the engine keeps verdict authority
  by validating the returned verdict against verifier-verdict-v1 fail-closed.
- **N-09**: OpenAI prefix rename lands with the new resolution (sworn#31):
  `openai/` → the Responses driver (modern default), `openai-completions/` →
  legacy chat/completions, `openai-responses/` kept as deprecated alias for one
  release.
- **N-10**: A codex exec subprocess driver ships alongside claude-cli
  (sworn#19) — the N=2 proof that the contract isn't claude-shaped; its own
  slice so it can late-defer cleanly if the release runs long.

## Constraints and non-negotiables

- Public-safe repo; no business/pricing/competitive content.
- Single Go binary, minimal justified deps; no provider SDKs (ADR-0007 / repo
  CLAUDE.md). The subprocess driver spawns CLIs; it adds no Go dependency.
- **Driver interface shape is architecturally significant ⇒ Type-1 (Rule 9)**:
  options + rationale recorded, human decision required; the model must not
  self-ratify it.
- The loop's in-engine verifier currently requires `ChatStructured` (only
  oai/openai-responses implement it) — any driver intended for the verifier
  role must either implement structured output or the seam must be redesigned
  deliberately (not by accident).
- No paid model dispatch during planning; live probes with stripped env only
  (sworn#69: `~/.sworn/.env` keys load silently — strip before probing).
- Planning artefacts follow track-mode flow; `board.json` validates against
  board-v1 and `index.md` is rendered from it (`sworn render`) — the drift
  guard is fail-closed now.
- Rule 11 applies to the subprocess driver by name: a git-bearing child process
  pointed at a worktree must fail-closed-assert the target directory exists and
  is the expected worktree before spawn.

## Adjacent / out of scope

- **Item**: S08-differential-validation (cross-engine parity vs the coach-loop
  reference). **Why deferred**: the reference is retired and
  schema-incompatible with baton v0.7.0; parity with a dead contract proves
  nothing forward. Dropped, not postponed — the validation intent is subsumed
  by the beefed-up S09 (conformance suite + engine-level SIT smoke).
  **Tracking**: S09's spec carries the SIT-smoke acceptance criteria; no
  forward issue needed for the archive-differential idea (archive stays at
  `~/projects/fired/baton-backup` if ever wanted). **Acknowledged**: Brad,
  2026-07-02, this session.
- **Item**: Baton v0.7.0 re-vendor. **Why deferred**: a live behaviour + data
  migration across multiple in-flight releases, deliberately not bundled with
  an architecture re-seam. **Tracking**: sworn#48. **Acknowledged**: Brad,
  2026-07-02 (pre-acknowledged in the starter capture).
- **FT-1 orchestration items** (serialized cold-start bootstrap, auto-WIP-commit,
  track-local failure isolation) — the 2026-06-28 plan already scoped these to a
  separate release; several landed via the operational-readiness releases.

## Decisions made during planning

### 2026-07-02 — Role-universality: every driver can serve every loop role it declares (A-01)

- **Context**: Should the subprocess driver serve the verifier role too, or
  does the verifier stay on ChatStructured-only drivers?
- **Options considered**: (a) yes, both roles this release; (b) split —
  implementer track first, verifier track depends_on it; (c) no, defer #55.
- **Decision**: Brad went past option (a): "Yes, arguably all the drivers
  should be able to be used for all the roles." Role-universality is a design
  principle of the contract, not a per-driver scope call. Any driver can serve
  any loop role it declares capability for; capability is per-role, checked
  fail-fast at resolution.
- **Why**: The queued dogfood (implementer sonnet / verifier opus via
  claude-cli) structurally requires it — cliDriver has no ChatStructured and
  `verify.RunAgentic` type-asserts it today. Serving verify through the driver
  also closes sworn#55 (verifier gets a real tool loop where the driver
  provides one). The engine keeps verdict authority: the driver returns the
  verdict, the engine validates it against verifier-verdict-v1 fail-closed.

### 2026-07-02 — Drop S08-differential-validation; S09 grows teeth (A-02)

- **Context**: S08's reference implementation (coach-loop) is retired and
  schema-incompatible with baton v0.7.0; an archive exists at
  `~/projects/fired/baton-backup`.
- **Options considered**: (a) drop S08, beef up S09; (b) keep S08 against the
  archive, pinning old schemas; (c) repurpose as pre/post-refactor golden-trace
  parity.
- **Decision**: (a) — drop S08. S09 becomes the per-driver conformance suite
  PLUS an engine-level SIT smoke: boot the ASSEMBLED `sworn loop` over a
  fixture release with a stub Driver and assert dispatch fires end-to-end.
- **Why**: The reference is dead code on a dead schema; parity with it proves
  nothing forward (the engine IS the loop now — no backport, per the
  2026-06-30 pivot). The SIT smoke wires in the §3.5 lesson — the test class
  that would have caught the nil-factory SIGSEGV and cold-start DOA.

### 2026-07-02 — Backlog consumption: #31, #19, #70 in; #15 folded into the registry design (A-03)

- **Context**: Which open backlog items land in this release vs stay tracked.
- **Decision**: All four selected. sworn#31 (openai/ prefix rename) lands with
  the new resolution — migrate the mapping once, not twice. sworn#19 (codex
  exec driver) ships as its own slice — the N=2 proof of driver generality,
  late-deferrable if the release runs long. sworn#70 (real-cost telemetry)
  lands in the telemetry slice — kill the $2/1M flat rate, wire the dark-code
  pricing registry, record subscription-CLI cost honestly (cost-source
  distinction, not fake $0 API spend). sworn#15 (self-registering factory) is
  not built as written — the driver registry replaces `NewClient`'s switch, so
  #15's problem dissolves; init()-vs-explicit-registration becomes a clause of
  the Type-1 interface decision (A-04).

## Schema-vs-spec audit notes

Live code-seam map (fresh Explore pass, 2026-07-02, `release/v0.1.0`) — the
facts the re-cut specs must be grounded in, where they diverge from the
2026-06-28 spec.md text:

- **Dispatch seam is factory-fields, not direct construction.** `RunSlice`
  dispatches via `RunSliceOptions.NewAgent/NewVerifier` factory fields with
  nil-defaults added by the 2026-06-28 eval supervisor fix
  (`internal/run/slice.go:57-63,193-201`); `internal/scheduler/worker.go` knows
  only an opaque `RunSliceFn` (`worker.go:96`); `cmd/sworn/run.go:134` builds
  the closure and relies on the nil-defaults. The old S05/S06 text ("replace
  NewAgent/NewVerifier direct use") predates this.
- **Verifier seam post-keystone.** `RunSlice` constructs the verifier as a full
  agent via `opts.NewAgent` (so it carries `ChatStructured`), then
  `verify.RunAgentic` type-asserts `model.StructuredOutput` and makes ONE
  `ChatStructured` call against `verifierEmitSchema`, semantic-gated by
  `baton.ValidateSchema("verifier-verdict-v1", ...)` (`internal/verify/verify.go:189-229`).
  Prose-scrape verdict parsing is deleted. Fail-closed: no StructuredOutput →
  INCONCLUSIVE.
- **Tool-loop ownership.** The agentic loop is `internal/agent.Run`
  (`agent.go:81`, max 25 turns, terminal on no-tool-calls); worktree
  confinement lives in the tool executor (`tools.go:29,321-323` —
  `cmd.Dir = root` for Bash/Grep, `HOME=root`, path prefix-confinement). Only
  `internal/implement` consumes it; the verifier never does (sworn#55).
- **cliDriver reality (sworn#35).** `claude -p --no-session-persistence --model
  <m> <prompt>`, `cmd.Stdin=nil`, **no `cmd.Dir`**, tools arg ignored, message
  history collapsed to one stacked prompt, output = trimmed stdout, cost/tokens
  always 0, `*exec.ExitError`→`KindAuth` (coarse), `CapVerify|CapChat`
  advertised (`internal/model/cli.go:19-157`). Codex: `ErrDriverNotImplemented`
  stubs at `cli.go:59-65` + `provider.go:175-180` (both cite sworn#19).
- **Two divergent resolution paths + a drifting third table.**
  `model.NewClient` (provider-prefix switch, `provider.go:87`) vs
  `model.FromEnv` (keyless-CLI first, then sworn-proxy routing, then direct,
  `config.go:40-53`); plus a hand-maintained `capabilityRegistry`
  (`registry.go:13`) used for `sworn capabilities`/`HasChat` that can drift
  from actual `Capabilities()` methods. S04's replacement must subsume all
  three or state which survive.
- **Capability matrix (live).** `ChatStructured` on exactly two types: `OAI`
  (`oai.go:318`) and `OpenAIResponses` (`openai_responses.go:240`). Anthropic
  Chat accepts-but-ignores tools (`anthropic.go:90-97`). Google/Bedrock/Azure/
  OCI/Ollama are Verify-only.
- **Two pricing systems, not cross-wired.** `pricing.go` `Pricing`/`ComputeCost`
  vs `client.go` `PriceForModel`/`ComputeCostFromTokens` (zero call sites —
  dark code, sworn#70); `agent.go:182` computeCost is nominal $2/1M flat;
  Anthropic's correctly computed `CostUSD` is discarded by the agent loop.
- **Telemetry record (live).** `state.Dispatch` already carries
  `DurationMS/InputTokens/OutputTokens/ModelIDConfirmed/Quadrant`
  (`state.go:83`) — the old S07 text ("add these fields") is stale; the gap is
  populating them honestly, not defining them.
- **Structured-outputs keystone location.** Wire layer
  `internal/model/structured.go` (strictProjection etc.); semantic layer
  `internal/baton/validate_schema.go` (draft-2020-12 via
  santhosh-tekuri/jsonschema/v6, embedded schemas in
  `internal/baton/schemas/`). There is no `internal/schema` package.

## Proposed slice decomposition (draft)

(Phase 3 — pending discovery decisions.)

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Should the subprocess driver serve the **verifier** role too? | N-08, track shape, verifier seam | RESOLVED 2026-07-02 — role-universality (see Decisions) |
| A-02 | S08 fate: archive-differential vs drop for beefed-up S09? | validation track | RESOLVED 2026-07-02 — dropped; S09 = conformance + SIT smoke (see Decisions) |
| A-03 | Which backlog items land in-release: #31/#19/#70/#15? | scope, touchpoints | RESOLVED 2026-07-02 — all in; #15 by subsumption (see Decisions) |
| A-04 | Driver interface shape (Type-1): exact `Dispatch` signature, per-role capability declaration, verdict seam, registration mechanism, fate of the single-shot `model.Verifier` used by non-loop gates. | every slice | options + rationale to human this session; recorded per Rule 9 |

## Screenshots / references

- (none yet)
