---
title: 'Release intake: 2026-06-16-fidelity-layer'
description: 'Discovery output for the fidelity-layer release — Baton Rules 8/9/10 (requirements, design, and customer-journey/system-acceptance fidelity) as protocol + native sworn enforcement.'
---

# Release Intake: `2026-06-16-fidelity-layer`

> Durable record of the requirements conversation. The slice list is downstream of this file.

## Release goal

Baton today verifies **delivery against the spec** rigorously (Rules 1/6/7: fresh-context,
fail-closed, adversarial). It treats the spec itself as an axiom. This release closes the
**front half and the top** of the fidelity chain that delivery verification cannot see:

- **Rule 8 — Requirements Fidelity (Seam 1):** is the spec the *right* spec? Requirements
  verification (quality), validation (does it make sense / serve the need), and enforced
  traceability — so a need cannot drop silently between intake and spec.
- **Rule 9 — Design Fidelity (Seam 2):** meeting a requirement is not the same as the right
  solution for the whole. A human-owned, stakes-calibrated design-fit gate, plus design-system
  conformance for UI-bearing projects.
- **Rule 10 — Customer-Journey Validation (Seam 3 / capstone):** all-slices-`verified` means
  every piece is locally correct against its own spec; it does **not** mean the assembled
  system works end-to-end. A release-level, human-witnessed acceptance gate over the critical
  customer journeys, mocks off, against real infra.

"Shipped" looks like: the three rules exist as Baton protocol (rule docs + role-prompt gates +
first-pass scripts) **and** as native `sworn` enforcement, each fail-closed, each proven on a
real release. The claim moves from *"delivered matches the spec"* to *"delivered matches what
you actually asked for, and the whole thing works."*

## Source of truth

- **Human stakeholder**: Brad (maintainer; 15+yr business-analysis background — requirements
  rigour is the design lens for this release).
- **Tracking issue / epic**: TBD — create a `sworn` epic issue before implementation (Rule 5).
- **Related public standards** (borrow, don't reinvent): ISO/IEC/IEEE 29148:2018 (requirement
  quality model); EARS (Easy Approach to Requirements Syntax) for acceptance criteria; BABOK v3
  verification-vs-validation + traceability (RLCM); SAFe/Scrum Definition of Ready / benefit
  hypothesis.
- **Detailed design rationale**: maintained privately by the maintainer (not in this public
  repo, per the repo's public-safe policy).

## Users and their gestures

The "user" of this release is the **developer or small team driving Baton/sworn**, and the
**agent roles** the protocol coordinates.

- **Solo founder / small-team developer (the planner-seat human)**: runs `/plan-release`
  interactively and is walked through requirements quality + positive/negative scenario
  sense-checks (Rule 8); makes stakes-gated design-fit decisions (Rule 9); walks the critical
  journeys at cutover and attests the system works (Rule 10).
- **Planner agent**: drafts EARS acceptance criteria, the requirements-quality first-pass, the
  RTM, and candidate customer journeys — for the human to ratify/adjust (AI-drafts /
  human-ratifies throughout).
- **Implementer / verifier agents**: consume a spec that is now verified + validated + traced;
  the verifier's oracle derives from the acceptance criteria, never the code.
- **Enterprise team (scalable depth)**: the same gates, with the RTM's vertical golden-thread
  (strategy → release → slice) carrying line-of-sight that enterprise governance needs.

## What's currently broken or missing

The fidelity gap, stated plainly:

- **No requirements verification.** Gate 0 checks a spec *has* its sections; it does not check
  each acceptance criterion is singular, unambiguous, complete, consistent, feasible — only
  "verifiable", one of the eight 29148 characteristics.
- **No requirements validation.** Slices carry no benefit/alignment hypothesis and no
  scenario sense-check, so there is nothing to validate the spec *against*.
- **No enforced traceability.** Intake items "become candidate acceptance checks downstream"
  in prose, but nothing enforces it. `need → AC` is unbound; items drop silently. The back half
  (`AC → test → proof`) is closed by the proof bundle; the front half leaks.
- **No design-fit gate.** Rule 7 proves "delivered matches spec", never "this was the right
  design for the product". UI-bearing projects also drift visually with no design-system input.
- **No system-level acceptance.** Per-slice verification is **local**; integration/composition
  defects live in the seams *between* slices, which no slice's spec owns. A board reading
  "READY TO SHIP" can still be broken end-to-end. The existing E2E tests are largely
  unit-level against stubs/mocks at the DB/auth/entitlement boundary — green proves the code
  works against a *mock of the world*, exactly where the showstoppers live.

## What the human wants

Discrete capabilities (candidate acceptance checks downstream):

- EARS-structured acceptance criteria, consistent and testable by construction.
- A requirements-quality gate over the 29148 characteristics (not just section presence).
- A **machine-checkable spec-quality first-pass**: soundness + completeness derived from the
  acceptance examples *before* any code (the requirements-side analog of the delivery
  first-pass script).
- A requirements-validation step that is **AI-drafts / human-ratifies**: walk the positive AND
  negative scenarios for each requirement; the human is the validation authority.
- A lightweight, enforced **RTM** — 2-D: horizontal `need → AC → test → proof` plus vertical
  golden-thread `strategy → release benefit → slice`. Lightweight-by-default, scalable to
  enterprise depth.
- A promoted Definition of Ready: `planned → in_progress` gates on "verified + validated +
  traced", not "sections present".
- A stakes-calibrated, human-owned design-fit gate (threshold = reversibility × blast-radius).
- A design-system input + conformance audit for UI-bearing projects (tokens + component
  library; deterministic first-pass + human judgement).
- Customer-journey elicitation (agents draft, human ratifies) as a durable platform artefact;
  per-release journey-impact analysis; a fail-closed human walkthrough at cutover with
  attestation; and journeys accreting into an automated regression suite over time.
- A read-only journey-validation **evidence surface** in `sworn top` (a green-board /
  kill-list), not a workflow manager.

## Constraints and non-negotiables

- **Fail-closed throughout.** Each new gate exits non-zero unless satisfied; exit 0 only on PASS.
- **Zero runtime dependencies.** Native enforcement stays stdlib-only Go (no new deps without
  an ADR).
- **Evidence, not workflow.** sworn produces and gates on *evidence*; it does not own
  phase-gating workflow (that stays the customer's tracker). The journey surface is read-only
  evidence.
- **Human stays the validation oracle.** Requirements validation (Rule 8) and system
  acceptance (Rule 10) are never LLM self-certified — AI drafts, the human ratifies. (Current
  research: spec validation has no oracle but the user; LLMs judging oracle correctness score
  near-random.)
- **Lightweight by default.** The RTM and the front-end gates must not over-proceduralise solo
  / small-team work; enterprise depth is opt-in, not the floor.
- **Public-safe.** No strategy/competitive/pricing content in release artefacts.

## Adjacent / out of scope

- **Item**: Safe, quality **parallelism** (dependency-aware concurrent track fan-out;
  process-ownership safety). **Why deferred**: it is the *next* release; front-end fidelity is
  the prerequisite that makes parallelism safe rather than dangerous (a weak spec yields N
  verified-wrong things, fast), so fidelity sequences first. **Tracking**: separate
  parallelism release; sworn#2 (process-ownership). **Acknowledged**: 2026-06-16.
- **Item**: A **design system for sworn itself**. **Why deferred**: sworn is a CLI; it has no
  UI to conform. The protocol must *support* a design-system input (Rule 9) for UI-bearing
  customer projects, but sworn ships none of its own. **Tracking**: Rule 9 (B2) supports it as
  a first-class input; sworn-CLI is exempt. **Acknowledged**: 2026-06-16.

## Decisions made during planning

### `2026-06-16` — One cohesive release, three tracks (not three releases)

- **Context**: the fidelity layer is three rules across two seams plus the capstone; could be
  one release or three.
- **Options considered**: three sequential releases (Rule 10 → 8 → 9); one cohesive release.
- **Decision**: one cohesive release, internally decomposed into three tracks.
- **Why**: stakeholder chose cohesion; scope-ceiling discipline is preserved at the **slice**
  level (each slice = one verifiable vertical), so cohesion does not mean an unverifiable blob.

### `2026-06-16` — Protocol + native together, protocol leads each slice

- **Context**: deliver the rules as Baton protocol docs, or as native Go `sworn` enforcement?
- **Options considered**: protocol-first then native later; native-first; both together.
- **Decision**: both together, with protocol leading each slice — design the gate, hand-prove
  it on a real release, then harden into Go.
- **Why**: a real-world hand-run is already generating evidence for the journey-validation gate;
  the spec should be driven by that lived prototype rather than guessed.

### `2026-06-16` — The ownership spine

- **Context**: where does each fidelity gate live, and who owns the judgement?
- **Decision**: the autonomous loop owns **delivery** fidelity (Rules 1/6/7); the interactive
  `/plan-release` + `/replan-release` surface owns **requirements + design** fidelity (Rules
  8/9, AI-drafts / human-ratifies); the **human at cutover** owns **system-acceptance** fidelity
  (Rule 10, fail-closed walkthrough). `/plan-release` and `/replan-release` are interactive by
  design; the rest of the commands are autonomous by default (and the interactive surface is
  the recommended on-ramp before turning automation on).
- **Why**: requirements + design fidelity are AI-drafts / human-ratifies by nature, which is
  exactly what an interactive session is for; system acceptance has no oracle but the human.

### `2026-06-16` — Plan + target on `release/v0.1.0` (continue the milestone line)

- **Context**: the first release merged into `release/v0.1.0`; sworn is pre-1.0.
- **Options considered**: continue on `release/v0.1.0`; cut `release/v0.2.0`; plan on `main`.
- **Decision**: continue on `release/v0.1.0` — treat it as the accumulating pre-1.0 milestone;
  the fidelity layer is the next release into it. Planning docs + specs commit here; the first
  `/implement-slice` branches `release-wt` + tracks from here.
- **Why**: v0.1.0 is the in-development milestone (not a shipped version), and the prior
  release's docs already live on this branch — one consistent home.

### `2026-06-16` — Spec Track C now (provisional), refine via `/replan-release`

- **Context**: Track C (Rule 10) draws its detailed design from a live journey-validation
  hand-run that is still running.
- **Options considered**: spec C now provisionally and refine later; defer C entirely to a
  `/replan-release` after the hand-run completes.
- **Decision**: spec all six Track C slices now at known fidelity, marking hand-run-dependent
  detail as explicit open items; refine via `/replan-release` as evidence lands.
- **Why**: keeps the release cohesive and the touchpoint matrix complete now (tracks are
  parallel-safe from day one); verified work stays immutable, so refinements append or take a
  new id rather than rewriting.

### `2026-06-16` — Spec-quality first-pass is its own slice (S03)

- **Context**: where does the soundness/completeness machine-checkable first-pass live?
- **Options considered**: own slice; folded into the 29148 requirements-verify gate (S04).
- **Decision**: own slice `S03-spec-quality-firstpass`.
- **Why**: it is a *deterministic* metric (mutation analysis over example I/O) — a different
  enforcement mode from S04's fresh-context 29148 judgement. It is also the single most
  directly implementable primitive (the requirements-side analog of the delivery first-pass
  script), worth isolating and verifying alone.

### `2026-06-16` — Native command surface: standalone verbs (primitive) + autonomous composition

- **Context**: how should the fidelity gates surface natively (S01/S03/S04/S09/S13/S15)?
- **Options considered**: standalone verbs; unified `sworn check`; decide per slice.
- **Decision**: **standalone verbs** (`sworn rtm`, `sworn ship`, `sworn top`, etc.) as the
  primitive — matching the existing `init/run/verify/bench` convention and mapping 1:1 to
  slash-commands for manual interactive driving (the on-ramp). The autonomous path **composes**
  the verbs (the run-loop / S06's DoR gate invokes the requirements checks at the
  `planned -> in_progress` transition); the gate *packages* (`internal/*`) are the shared
  primitive both paths call. An optional `sworn check` convenience that delegates to the verbs
  can be added later — not sliced now.
- **Why**: no strong counter-argument to verbs; the unified option's only advantage (one CI
  entry) is recovered by loop-side composition without losing the slash-command mapping. Verb
  proliferation is a future namespacing watch-item, not a current blocker.

### `2026-06-18` — Lint namespace: `sworn lint <target>` supersedes bare verbs for quality gates

- **Context**: after S01 (`sworn rtm`) and S02 (`sworn ears`) were implemented, both command
  names were found to be opaque in code review. `ears` is borrowed jargon (EARS = Easy Approach
  to Requirements Syntax) that means nothing without knowing the spec. `rtm` is an acronym
  (Requirements Traceability Matrix) equally opaque to newcomers. A user reading `sworn ears` or
  `sworn rtm` for the first time has no affordance for what it does.
- **Options considered**: keep bare verbs; rename to descriptive verbs (`sworn validate-acs`,
  `sworn check-trace`); group under a `lint` namespace.
- **Decision**: **`sworn lint <target>`** as the command surface for all quality-checking gates.
  - `sworn lint ac <release>` — acceptance-criteria format validation (was `sworn ears`)
  - `sworn lint trace <release>` — traceability matrix (was `sworn rtm`)
  - Future gates follow the same pattern: `sworn lint spec`, `sworn lint design`, etc.
  - `sworn lint` (no args) — future: run all targets
- **Why**:
  - `lint` is immediately understood by any developer — `golint`, `eslint`, `ruff`, etc. all
    share the same mental model: "check that this file/project is well-structured."
  - `ac` and `trace` are plain English targets — no prior knowledge of EARS or RTM required.
  - The namespace makes the extension path obvious; adding a new quality gate is `sworn lint <new>`, not a new top-level verb.
  - `trace` is a true verb ("check the trace"), unambiguous without the RTM acronym.
  - Internal packages (`internal/ears`, `internal/rtm`) keep their precise names — only the
    user-facing CLI surface changed. No internal knowledge is lost; it's a presentation decision.
- **Supersedes**: the 2026-06-16 "standalone verbs" decision for quality-gate commands.
  Non-quality verbs (`sworn run`, `sworn verify`, `sworn init`, `sworn bench`) are unaffected.
- **Tracked in**: S16-lint-rename (documentation sweep + proof bundle restoration).

**House style (confirmed):** acceptance checks are written in **EARS**
(`WHEN/WHILE/IF/WHERE … THE SYSTEM SHALL …`) from slice one (dogfooding S02). Specs hold
**tight scope seams** — each slice owns one mechanism; adjacent concerns are explicit
out-of-scope pointers to their own slice.

### `2026-06-16` — Track grouping: core + three dependent lanes (not three rule-tracks)

- **Context**: the rule-clean A/B/C split is **not** touchpoint-disjoint — all three
  front-end gates route through `internal/prompt/planner.md`, and the RTM keystone (S01) writes
  the shared native core (`internal/state/state.go`, `internal/board/index.go`) that the
  journey + evidence slices also touch.
- **Options considered**: ratify a core + three dependent lanes; one sequential track; split
  the core further.
- **Decision**: **T1 fidelity-core → T2 ∥ T3 ∥ T4**, each lane `depends_on` T1.
  - **T1 fidelity-core**: S01, S02, S04, S05, S07, S11 (planner.md, state, board, templates,
    requirements packages, journey-create).
  - **T2 delivery & cutover**: S06, S10, S12, S13, S14 (implementer.md, verify, state
    transitions, `sworn ship`, journey walkthrough/regression).
  - **T3 leaf-gates (deterministic)**: S03, S08, S09 (specquality, designaudit, config, bin
    scripts).
  - **T4 evidence-surface**: S15 (`sworn top`, board read).
- **Why**: the interactive surface is the spine, so the front end cannot fan out into three
  independent rule-lanes; a core-then-fan-out shape is the honest touchpoint-disjoint structure.
  Aggressive parallel safety is the *next* release's job — this release stays correctly gated.

**Cross-track convention (recorded):** `cmd/sworn/main.go` carries an **additive command
switch** — each command-adding slice (S01 `rtm`, S13 `ship`, S15 `top`) contributes a distinct
`case`. Per the established convention in the prior release (parallel tracks each added a case),
additive command registration in `main.go` is **not** treated as a touchpoint collision. New
command *implementations* live in their own `cmd/sworn/<cmd>.go` files (disjoint).

## Schema-vs-spec audit notes

- The RTM is not a new store — it threads through the **existing** artefacts: intake needs →
  `spec.md` acceptance checks → `required tests` → `proof.md`. It plugs into the proof bundle,
  which already closes `AC → test → proof`. The RTM adds the *front* half (`need → AC`) and the
  *vertical* axis (strategy → release → slice) over the same chain. Confirm against the live
  `status.json` / proof-bundle shape before specifying the trace fields (A1).

## Proposed slice decomposition (draft)

> Ratified via the Scope-Ceiling Bar + Dependency Graph + track grouping in Phase 3/3b below.
> Three tracks; ~14 slices. Track C's detailed specs firm up from a live hand-run of the
> journey-validation gate (conducted separately).

**Track A — Rule 8: Requirements Fidelity**
- `S01-rtm-spine` — 2-D traceability matrix threaded through intake/spec/proof (keystone).
- `S02-ears-ac-format` — EARS acceptance-criteria template + validator.
- `S03-spec-quality-firstpass` — soundness + completeness metrics computable pre-code.
- `S04-requirements-verify-gate` — 29148 quality-characteristic check (fresh-context).
- `S05-requirements-validate-gate` — scenario pos/neg sense-check + benefit/alignment hypothesis.
- `S06-definition-of-ready` — promote Gate 0 to verified+validated+traced.

**Track B — Rule 9: Design Fidelity**
- `S07-design-fit-gate` — stakes-calibrated, human-owned design decision (reversibility × blast-radius).
- `S08-design-system-input` — design system (tokens + component library) as first-class input.
- `S09-design-conformance-audit` — deterministic first-pass + human judgement.

**Track C — Rule 10: Customer-Journey Validation**
- `S10-no-mock-boundary` — fail-closed on environment; an agent that can't reach real infra stops, never mocks around.
- `S11-journey-elicitation` — AI drafts critical journeys, human ratifies (durable artefact).
- `S12-journey-impact-analysis` — per-release: which journeys a release touches = its validation scope.
- `S13-walkthrough-attestation` — fail-closed human walkthrough + attestation at cutover.
- `S14-journey-regression-suite` — validated journeys accrete into an automated regression suite.
- `S15-sworn-top-evidence` — read-only journey-validation status surface in `sworn top`.

## Open questions

> Phase 2/3 decision points are all resolved above (Track C timing, S03 split, native command
> surface). Remaining genuine unknowns, to be closed before the dependent slices leave `planned`:

- **Track C detail pending the live journey-validation hand-run.** S11 (journey-elicitation
  schema), S12 (impact-analysis heuristic), S13 (walkthrough/attestation format) are specced
  provisionally; their hand-run-derived detail is refined via `/replan-release`. Each Track C
  spec names its own provisional sections.
- **Trace id scheme (S01)** — the exact stable-id convention for intake needs is an
  implementation choice the implementer fixes; the spec mandates *stability*, not the format.

## Screenshots / references

- (none yet)
