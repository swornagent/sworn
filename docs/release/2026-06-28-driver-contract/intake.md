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
- **N-11**: Per-provider model catalogs (decided in, 2026-07-02) — for each
  provider the user has linked (OpenRouter, Google, Mistral, Groq, ...), the
  system lists the models actually available on that account, with per-model
  capability info where the provider reports it over the wire and an honest
  "unknown" where it doesn't. Unknown is never treated as capable
  (fail-closed). Own slice; late-deferrable; active probing out of scope.
- **N-12 (added 2026-07-02; re-aimed v0.9.0 2026-07-06; re-aimed v0.10.0 2026-07-11)**: sworn re-vendors Baton at the
  **v0.10.0** tag (commit a5ab2aa) — both embed roots re-synced; the spec writer emits and the
  strict reader accepts `in_scope`/`out_of_scope`; the quadrant enum adopts
  `quick` for `chore` and `beast` for `epic` in code (via a read-path normalise shim,
  Validate strict — D1 2026-07-11); `acknowledged_by` round-trips;
  (sworn#80) a track's worktree path + state are derived from git refs, not
  persisted to board.json (board-v1 pure plan); AND the two new v0.10.0 schemas
  (contracts-v1, assembly-proof-v1) are vendored ADVISORY-only into both embed
  roots — the code half of sworn#48. Baton shipped v0.10.0 mid-build 2026-07-11;
  revendoring to the now-stale v0.9.0 would reopen the schema-skew scar.
- **N-13 (added 2026-07-02; re-aimed to v0.9.0 2026-07-06)**: every live release record on the
  integration branch conforms to the v0.9.0 contract — quadrant data migrated
  `chore`→`quick` and `epic`→`beast` across all releases, `in_scope`/`out_of_scope` present on
  every spec.json (empty arrays for historical records), every board.json migrated to the
  pure-plan shape (tracks[].state + worktree dropped), the one invalid
  `feature` quadrant fixed, renders refreshed, reader tightened to the v0.9.0 enum —
  the data half of sworn#48.
- **N-14 (added 2026-07-06, driver-contract replan)**: `sworn regress` runs the Go
  test suite from the module directory (the dir containing `go.mod`), not the
  worktree root — so a repo whose Go module lives in a subdirectory (e.g. `<repo>/go`)
  no longer reports a spurious `go test` setup failure. Surfaced downstream during
  fired's `/merge-track` (capture brief Defect 1).
- **N-15 (added 2026-07-10, driver-contract replan; inbound packet from fired)**:
  the runner treats BLOCKED as terminal-for-the-lane — a dispatched implementer's
  explicit `blocked` return or a verifier BLOCKED verdict stops all further
  dispatches for that slice in the run, halts the owning sequential track, consumes
  no retry budget, and the exit report distinguishes halted-BLOCKED (routed to
  `/replan-release`, blocker text verbatim) from exhausted-FAIL — so the loop never
  burns dispatches against a blocker only a replan can clear. FAIL keeps existing
  retry semantics unchanged. Evidence: fired's 2026-07-10 one-CP run dispatched
  three implementers against an unchanged spec defect (two wasted full-context
  dispatches). Origin packet: consumer repo,
  `apps/docs/content/docs/captures/2026-07-10-sworn-blocked-terminal-slice-packet.md`.
  DELIVERED by S14-blocked-terminal (verified+merged); this is W1 of the
  2026-07-11 contract-edge handoff (sworn#88, closed 2026-07-11).
- **N-16 (added 2026-07-11, contract-edge replan; W2 part 2)**: `sworn doctor`
  declares which Baton schema versions this binary grades — an explicit
  graded-schema-version manifest (graded vs vendored-advisory, including the new
  contracts-v1 / assembly-proof-v1) — so the next protocol/runner skew surfaces as
  a VISIBLE doctor warning, not a silent divergence under identical `$id`
  (the baton#54/#55/#58 scar). Split into sibling slice S15-baton-version-handshake
  in T7. Source: `docs/captures/2026-07-11-contract-edge-step3-handoff.md` (W2),
  `docs/captures/2026-07-11-replan-driver-contract-contract-edges.md`.
  NOTE: the contract-edge GRADERS (W3 `sworn lint contracts`, W4 `sworn assemble`)
  are a separate follow-on release (Coach 2026-07-11), NOT scoped here.

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
  **REVERSED 2026-07-02 (same day, second planning pass)**: Brad asked to roll
  sworn#48 into this release. Now in scope as track T7-baton-revendor
  (S11 code half, S12 data half), sequenced last (depends_on T4+T5+T6) so the
  repo-wide record migration runs when no sibling track of this release holds
  diverged copies. See the decision entry below.
- **FT-1 orchestration items** (serialized cold-start bootstrap, auto-WIP-commit,
  track-local failure isolation) — the 2026-06-28 plan already scoped these to a
  separate release; several landed via the operational-readiness releases.
- **Item**: model-backed spec checks (`sworn reqverify`, spec-ambiguity
  LLM check) not run at planning close. **Why deferred**: the planning session
  ran under the no-paid-dispatch constraint (starter capture; sworn#69 —
  `~/.sworn/.env` keys load silently). **Tracking**: run them as the first
  Definition-of-Ready step before any slice moves `planned → in_progress`,
  together with `sworn reqvalidate` (the human-ratified validation records are
  also still to be written — a Rule 8 DoR step, not a planning-close step).
  **Acknowledged**: Brad, 2026-07-02 (this session's handoff message).
- **Item**: `sworn lint ac` (exit 2) and `sworn specquality` (exit 1) fail on
  this release. **Why not fixed here**: pre-existing tooling gap, not a record
  defect — both readers still parse `spec.md`, which canonical (spec-v1-only)
  releases do not have; verified by identical failures on the sibling
  `2026-07-01-release-hygiene`. Known family: the render-drift release's
  deferred "spec.md-only parsers" item; do NOT manufacture spec.md files
  (memory: `feedback_releaseverify_specmd_false_fail`). The load-bearing gates
  pass: `sworn lint trace` (11 needs, PASS), `sworn designfit` (PASS),
  `sworn board`, `sworn render`. **Tracking**: fold into the existing
  spec.md-parser migration backlog (render-drift intake deferral).
  **Acknowledged**: Brad, 2026-07-02 (this session's handoff message).

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

### 2026-07-02 — Driver contract shape: role-dispatch (A-04, Type-1, part 1 of 4)

- **Context**: The exact contract every driver implements — the
  architecturally-significant (Type-1) choice of the release.
- **Options considered**: (a) role-dispatch — one
  `Dispatch(ctx, DispatchInput{Role,...}) (Result, error)` with drivers
  declaring a `RoleSet`; (b) minimal core + optional interfaces discovered by
  type-assert (today's RunAgentic pattern generalised); (c) maximal — fold
  non-loop utility judgements in too.
- **Decision** (Brad): **(a) role-dispatch.** Sketch as presented and chosen:
  `Driver{ Name(); Roles() RoleSet; Dispatch(ctx, DispatchInput) (Result, error) }`;
  `DispatchInput{ Role, ModelID, SystemPrompt, Payload, WorktreeRoot,
  VerdictSchema, Timeout }`; `Result{ Status(ok|blocked|error+Kind),
  ResultText, StructuredJSON, CostUSD, CostSource, InputTokens, OutputTokens,
  ModelID, DurationMS }`. Capability IS the declared role set, checked
  fail-fast at resolution. The engine keeps verdict authority by validating
  `Result.StructuredJSON` against verifier-verdict-v1 fail-closed. Wire types
  (ChatMessage/ToolDef) become internal to in-process drivers.
- **Why**: Matches the role-universality decision exactly; kills the sworn#35
  "advertises Chat but ignores tools" class at resolution time instead of
  runtime; accepted cost = the largest rewire of the verify.go/slice.go seams.
- **Rule 9**: Type-1, human-decided this session; to be recorded as the design
  decision in the owning slice's `status.json` when that slice is cut.

### 2026-07-02 — Model→driver resolution: explicit prefix, no smart fallback (A-04, part 4 of 4)

- **Context**: How a model ID resolves to a driver.
- **Options considered**: explicit prefix→driver always, vs smart fallback
  (e.g. `anthropic/opus` → claude-cli when keyless).
- **Decision** (Brad): explicit prefix. "System must make it easy for the user
  to know what's available" — discoverability is solved by listing, not by
  silent rerouting. A missing CLI/key fails fast at resolution naming the fix.
  The sworn#31 rename is part of this mapping.
- **Why**: Auditable routing (the same model ID must dispatch through the same
  code path on every machine, or eval telemetry is mud); the sworn#69 lesson —
  silent rerouting is a defect class, not a convenience.
- **Follow-on requirement surfaced**: automatic per-provider model catalogs
  (see N-11 draft + ambiguity A-05).

### 2026-07-02 — One-shot path is utility-only (A-04, part 2 of 4)

- **Context**: Fate of the single-shot `model.Verifier` interface. The
  stateless LOOP verifier was already removed by the keystone; what remains on
  the interface is non-loop usage.
- **Decision** (Brad, confirming the proposed reading): loop roles
  (implement / verify / captain) go through `Driver.Dispatch`, always. The
  one-shot interface survives strictly as the utility judgement path —
  `sworn verify`, `reqverify`, `llm-check`, `bench`, and orchestrator quick
  checks (text→text, no worktree). Keyless/Verify-only providers keep serving
  gates untouched.
- **Why**: Role dispatch and utility judgement are different jobs; folding the
  gates onto a worktree-shaped contract adds touchpoints for no user-visible
  gain. Two vocabularies persist, accepted.

### 2026-07-02 — Registration: explicit table + enumeration API (A-04, part 3 of 4; closes the #15 question)

- **Context**: How drivers register. Brad asked whether the binary can
  "iterate through the installed list" instead of a shared file.
- **Finding surfaced**: Go has no usable runtime discovery for compiled-in
  code (the `plugin` package is platform/version-locked; the single-static-
  binary rule excludes it) — `init()` self-registration is still compile-time,
  it just hides the list in import side-effects. A true "installed list" is
  only possible for subprocess drivers via a git-style `sworn-driver-*`
  executable convention — a future wire-protocol release, and the contract
  must not preclude it.
- **Decision** (Brad): explicit table — one `DefaultRegistry(cfg)` constructor
  wiring the ~4 compiled-in drivers; the registry exposes enumeration +
  per-driver availability probing (CLI on PATH? key present?), which is the
  machinery the catalog UX needs. sworn#15 closes by subsumption: the
  NewClient switch it targeted is replaced by the registry; the collision
  concern shrank from 14 provider files to ~4 drivers.
- **Why**: Deterministic, auditable, config flows in explicitly (no init()
  package-level state); one shared line per new driver is an accepted cost.

### 2026-07-02 — Model catalog lands in-release as its own slice (A-05, resolves N-11)

- **Context**: Explicit-prefix resolution trades away magic; discoverability
  must be first-class. Brad: "the system must make it easy for the user to
  know what's available" — per linked provider, list the models reported over
  the wire.
- **Decision** (Brad): in-release, own slice (late-deferrable). A
  `sworn models` affordance: per linked provider, the model list from the
  provider's models endpoint, annotated with wire-reported capabilities
  (OpenRouter `supported_parameters` incl. tools; Mistral `capabilities`;
  Ollama `/api/show` capabilities; Google `supportedGenerationMethods`
  partial) and an honest `unknown` for bare-list providers
  (OpenAI/Groq/Anthropic). Fail-closed: unknown ≠ capable.
- **Why**: Completes the explicit-prefix bargain; rides the same registry
  seam. Active capability PROBING (paid test calls per model) stays out of
  scope regardless — probing is only ever an explicit command, never
  automatic.

### 2026-07-02 — sworn#48 rolled in as T7; in_scope/out_of_scope backfilled (second planning pass)

- **Context**: The baton skill/docs update session surfaced that public spec-v1
  now requires `in_scope`/`out_of_scope` (dc3b7cc, post-v0.7.0-tag) and that
  sworn's pin is still v0.6.3. Brad: backfill the new fields into this
  release's specs AND roll sworn#48 into the same release.
- **Decision** (Brad): (1) all 12 spec.json records now carry real
  `in_scope`/`out_of_scope` content (backfilled from the intake decisions and
  each spec's rationale — not empty arrays). (2) sworn#48 lands as
  **T7-baton-revendor**: `S11-baton-revendor` (pin bump to v0.7.1, schema
  re-sync of both embed roots, writer/reader adoption, quadrant `quick` in
  code with transitional `chore` tolerance, acknowledged_by round-trip) then
  `S12-record-migration` (repo-wide record sweep `chore`→`quick`, presence
  backfill for historical specs, the invalid `feature` quadrant fix, render
  refresh, tolerance removed).
- **Why sequenced last** (depends_on T4+T5+T6): the data sweep touches every
  release's records including this one's; running it after all sibling tracks
  merge means no diverged in-track copies of THIS release's records exist
  (the 2026-06-28 replan-propagation lesson). The two slices share one track
  so the required-fields schema flip and the record backfill merge as a unit —
  the integration branch never sees the intermediate state.
- **Preconditions surfaced**: upstream tag v0.7.1 must be cut and pushed from
  baton main (cd42ca1 — includes dc3b7cc in_scope/out_of_scope and today's
  scope→user_outcome prose fix; cd42ca1 is currently local-only). S11 BLOCKs
  on this rather than pinning an untagged SHA.
- **Out of scope confirmed**: board-v1/proof-v1 canonical conformance stays
  excluded (baton#54), exactly as sworn#48 states.

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

## Proposed slice decomposition (approved 2026-07-02)

10 slices, 6 tracks; approved by Brad via decision cards (see Decisions). The
2026-06-28 spec.md-era slices are superseded and removed by the re-cut (raw
material preserved in git history at commit 7c49f51 and earlier).

- `S01-driver-contract` — the role-dispatch Driver contract types + ADR-0012;
  owns the Type-1 design-decision record. (T1)
- `S02-claude-subprocess-driver` — claude-cli subprocess driver, both roles,
  cmd.Dir + Rule-11 assert + JSON envelope parsing; the sworn#35 fix. (T2)
- `S03-codex-subprocess-driver` — codex exec variant; closes sworn#19;
  late-deferrable. (T2)
- `S04-inprocess-oai-driver` — agent loop + OAI/Responses clients behind the
  contract; agentic verify = tool loop then structured verdict. (T3)
- `S05-driver-registry` — DefaultRegistry(cfg) explicit table + Resolve
  fail-fast + enumeration/availability + sworn#31 prefix rename. (T4)
- `S06-loop-dispatch-rewire` — RunSlice all role legs via Dispatch; factories
  deleted; engine-side verdict validation; import-boundary test. (T4)
- `S07-scheduler-failfast` — parallel loop resolves all role×model at startup,
  fails fast named; factory helpers deleted. (T4)
- `S08-honest-cost-telemetry` — sworn#70: unify pricing, kill $2/1M flat,
  CostSource honesty (cli/subscription/table/unknown). (T4)
- `S09-model-catalog` — `sworn models` per-provider listing with wire-reported
  capability annotations; unknown ≠ capable. (T5)
- `S10-conformance-sit` — exported driver conformance suite + SIT smoke
  booting the assembled loop over a fixture release with a stub driver. (T6)
- `S11-baton-revendor` — pin bump to baton v0.7.1, schema re-sync, writer/
  reader adoption of in_scope/out_of_scope, quadrant `quick` in code,
  acknowledged_by round-trip (sworn#48 code half; added second pass). (T7)
- `S12-record-migration` — repo-wide record sweep chore→quick + presence
  backfill + `feature` quadrant fix + render refresh + quick-only tightening
  (sworn#48 data half; added second pass). (T7)

### 2026-07-02 — Decomposition + track grouping approved

- **Context**: Phase 3/3b — slice list and parallelism structure.
- **Decision** (Brad): 10 slices as listed above; 6 tracks
  T1-contract(S01) → T2-subprocess(S02→S03) ∥ T3-inprocess(S04) →
  T4-resolution-loop(S05→S06→S07→S08, depends_on [T2,T3]) →
  T5-catalog(S09) ∥ T6-proof(S10) (both depends_on [T4]).
- **Why**: The middle is honestly serial (T4 is the spine of the re-seam);
  parallelism is real at the driver-implementation pair and the closing pair.
  depends_on edges legalize the file overlaps between dependent tracks (e.g.
  T4's `internal/agent/agent.go` cost removal lands after T3 merges its
  wrapping of the same package). If S03-codex defers late, T2 shortens and
  nothing re-groups.

## Ambiguity register

| # | Ambiguity | Affects | Resolution |
|---|-----------|---------|------------|
| A-01 | Should the subprocess driver serve the **verifier** role too? | N-08, track shape, verifier seam | RESOLVED 2026-07-02 — role-universality (see Decisions) |
| A-02 | S08 fate: archive-differential vs drop for beefed-up S09? | validation track | RESOLVED 2026-07-02 — dropped; S09 = conformance + SIT smoke (see Decisions) |
| A-03 | Which backlog items land in-release: #31/#19/#70/#15? | scope, touchpoints | RESOLVED 2026-07-02 — all in; #15 by subsumption (see Decisions) |
| A-04 | Driver interface shape (Type-1): `Dispatch` signature, per-role capability, verdict seam, registration, one-shot-path fate. | every slice | RESOLVED 2026-07-02 in four parts (see Decisions): role-dispatch contract; one-shot utility-only; explicit table + enumeration; explicit prefix |
| A-05 | Model catalog: in-release vs follow-on; honesty of capability filtering on heterogeneous provider metadata. | N-11, scope | RESOLVED 2026-07-02 — in-release, own slice, fail-closed unknowns, probing excluded (see Decisions) |

## Screenshots / references

- (none yet)
