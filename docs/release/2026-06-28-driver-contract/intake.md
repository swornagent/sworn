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
- **N-08 (open)**: The verifier role dispatches through a driver with a real
  tool loop (subprocess CLI re-runs tests in the worktree) — closes sworn#55.
  Pending human decision (see ambiguity register A-01).

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

(To be confirmed during discovery — candidates below; each needs why +
tracking + acknowledgement before the board closes.)

- **Baton v0.7.0 re-vendor** — tracked sworn#48; a live behaviour + data
  migration across multiple releases, deliberately not bundled here.
- **FT-1 orchestration items** (serialized cold-start bootstrap, auto-WIP-commit,
  track-local failure isolation) — the 2026-06-28 plan already scoped these to a
  separate release; several landed via the operational-readiness releases.

## Decisions made during planning

(Appended chronologically as AskUserQuestion decision points resolve.)

## Schema-vs-spec audit notes

- The 2026-06-28 cut's `Result` shape (`Status ok|blocked|error`, `Verdict`,
  `CostUSD`, tokens, `ModelID`, `DurationMS`) predates verifier-verdict-v1.
  The verify path now returns a schema-validated verdict object via
  `ChatStructured` — the re-cut spec for the Result/verdict seam must be
  grounded against the live `internal/verify` + `internal/schema` code, not the
  old spec.md text. (Code-seam map in progress; findings land here.)

## Proposed slice decomposition (draft)

(Phase 3 — pending discovery decisions.)

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Should the subprocess driver serve the **verifier** role too (CLI re-runs tests itself in the worktree), closing sworn#55 in this release — or does the verifier stay on ChatStructured-only drivers for now? | N-08, track shape, verifier seam | human will decide during discovery (this session) |
| A-02 | S08-differential-validation's reference (coach-loop) is retired + schema-incompatible with baton v0.7.0. Differential-validate against the archive (`~/projects/fired/baton-backup`, pinning old schemas) vs drop S08 in favour of a beefed-up S09 conformance suite? | validation track | human will decide during discovery (this session) |
| A-03 | Which backlog items land IN this release vs stay tracked: #31 (prefix rename), #19 (codex driver), #15 (self-registering factory — full form vs driver-registry subsumes it)? | scope, touchpoints | human will decide during discovery (this session) |
| A-04 | Driver interface shape (Type-1): exact `Dispatch` signature, capability declaration, structured-output seam for the verifier. | every slice | options + rationale to human after code-seam map; recorded per Rule 9 |

## Screenshots / references

- (none yet)
