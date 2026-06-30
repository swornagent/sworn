---
title: Baton — portable engineering rules + harness for AI-assisted projects
description: A self-contained, portable set of rules, role prompts, templates, and brainstorm patterns for keeping AI-assisted engineering work rigorous, durable, and free from "dark code" / "silent deferral" / "ephemeral knowledge" / "completion overclaiming" failure modes.
---

# Baton

A portable, project-agnostic process package for engineering teams working with AI coding agents (Claude Code, Cursor, Copilot, Gemini CLI, etc.).

The package contains four kinds of artefact, in increasing order of operational specificity:

1. **Rules** (ten, listed below) — the underlying engineering discipline. Adoptable independently or together.
2. **Role prompts** (planner, implementer, verifier, captain) — paste-into-session prompts that operationalise the rules for a specific role in a release. The **Captain** runs design-review (Rule 9). Authority over strategy and product/architecture sits with the **Coach** — the human-in-the-loop who owns the team; the agent roles surface decisions to the Coach but never self-authorise them.
3. **Templates** (slice folder, release intake, release board) — fillable artefacts that become the durable record of a release.
4. **Brainstorm patterns** (Option Matrix, Decision Card, Scope-Ceiling Bar, Dependency Graph, Deferral Card) — visual decision surfaces that make planning decisions discrete, visible, and capturable.

Rules 1-5 can be adopted on their own; the rest of the package is needed only when Rules 6–11 (Proof Bundle through Process-Global Mutation Guard — the harness-enforced rules) are in play. See `INSTALL.md` for adoption paths.

## Core principle — DRY applied bidirectionally to knowledge

Don't Repeat Yourself is usually stated about code. Baton extends it to **all knowledge artefacts** — decisions, analysis, rationale, context — and applies it **in both directions**:

- **Forward DRY**: when creating new knowledge, capture it once in the most durable layer available. Link from elsewhere; don't duplicate.
- **Backward DRY**: don't let prior knowledge be lost and then recreated. The audit you ran yesterday should not be re-run next week. The decision you made in chat should not be re-litigated next session. The rationale you put in a plan doc should not be reconstructed by reading the diff six months from now.

Knowledge that survives both directions of DRY is **codified, organised, traceable, and auditable**:

- **Codified** — captured in a stable form (code, commit message, doc, issue, memory).
- **Organised** — findable when next needed, without grep-luck.
- **Traceable** — origin and decision path are visible, not inferred.
- **Auditable** — verifiable from external evidence, not from someone's recollection.

The eleven rules below are facets of this principle. Each rule prevents one specific way knowledge gets repeated, lost, or re-litigated.

## Why this matters — insulation against requirements failure

Decades of post-mortems across software projects converge on a single dominant cause of project failure: **poor requirements**. Not poor coding, not poor architecture, not insufficient testing — requirements that were lost, drifted, miscommunicated, silently downgraded, met technically but missed the intent, or never explicitly captured at all.

Bidirectional DRY on knowledge is structural insulation against this:

| Requirements failure mode | How Baton insulates |
|---|---|
| Requirements lost between elicitation and implementation | Rule 5 (Session Discipline) anchors work to a durable tracker. Rule 3 (Capture) preserves analysis. |
| Requirements drift silently as the project progresses | Rule 2 (No Silent Deferrals) makes silent drift impossible — drift requires explicit acknowledgement. |
| Requirements met technically but miss the intent | Rule 1 (Reachability Gate) verifies the user-facing affordance, not just the underlying primitive. |
| Requirements interpreted differently by different stakeholders | Rule 4 (Commit Messages) keeps rationale and intent attached to the diff, traceable to its origin. |
| Requirements never explicitly captured | Rule 3 (Capture) + Rule 4 (Commit Messages) together make capture mandatory at multiple checkpoints. |
| Completion claimed against intended state rather than actual repo state | Rule 6 (Proof Bundle) forces completion to be backed by `git diff` + test output written to disk. |
| Completion self-attested by the same reasoning thread that did the work | Rule 7 (Adversarial Verification) routes verification through a fresh-context session that never saw the implementer's framing. |

The four properties of well-captured knowledge — **codified, organised, traceable, auditable** — map directly onto the IEEE 29148 / ISO 25010 requirements-quality criteria (verifiability, traceability, consistency, completeness). The package doesn't formalise requirements engineering; it ensures the artefacts produced by *any* requirements process actually survive the implementation cycle.

This is the strongest case for adoption by any org that cares about reducing project risk: you don't need to change your requirements methodology, your tooling, or your team structure. You add a thin layer of knowledge-preservation rigour underneath whatever you already do, and the largest single class of project failure becomes structurally harder.

## The failure modes this addresses

AI-assisted engineering has a small number of recurring failure modes that compound into substantial rework and churn:

1. **Dark code** — components / modules / hooks built and unit-tested but never wired to a user-reachable path. Tests pass green; feature is unreachable. *(The cost is repetition: the primitive gets re-discovered later as a gap and re-built, or re-tested, or re-investigated.)*
2. **Silent deferrals** — inline `// deferred` / `// later` / `// TODO` comments rationalised as decisions without the user / decision-maker ever being told. *(The cost is repetition: future sessions re-evaluate what was supposedly already decided.)*
3. **Ephemeral knowledge** — analysis, audits, design decisions that live only in conversation context and are lost on session boundaries. *(The cost is repetition: the same audit gets re-run by future sessions because the original is unreachable.)*
4. **Untracked decisions** — choices made in chat that don't make it into commits, docs, or issues; future sessions re-litigate them. *(The cost is repetition: the same decision gets made multiple times, sometimes inconsistently.)*
5. **Completion overclaiming** — the implementer's session declares the slice done while only a thin slice actually landed; the next session's stocktake re-discovers the gap. *(The cost is repetition: the work that wasn't done gets re-planned, re-scoped, and re-attempted.)*
6. **Self-certification** — the implementer's reasoning thread also writes the proof bundle. Optimism contaminates the certification. *(The cost is repetition: gaps appear in verification rather than implementation, and propagate downstream.)*

Every failure mode is a violation of bidirectional DRY on knowledge. This package codifies eleven rules that intervene at each failure mode's source. Adopt them whole, in fragments, or as a starting point for your own rule-set.

## The eleven rules

| Rule | What it does | Forward DRY ↔ Backward DRY | File |
|---|---|---|---|
| 1. Reachability Gate | TDD's failing test must render through the user-path integration point, not the leaf component. Component imported only by its own test is a red flag. | Build the primitive once, in a place that's reachable ↔ Don't re-build dark primitives | `reachability-gate.md` |
| 2. No Silent Deferrals | Inline "deferred" comments require *why* + *tracking* + *acknowledgement*, or they are rationalisations not decisions. | Decide once, surface the decision ↔ Don't re-litigate forgotten decisions | `no-silent-deferrals.md` |
| 3. Capture Discipline | Conversation context is the most ephemeral persistence layer. Subagent findings + session decisions must land in durable storage (docs / issues / commits / memory) before session ends. | Capture analysis once to durable storage ↔ Don't re-run the same audit | `capture-discipline.md` |
| 4. Commit Messages as Capture Layer | Commits that land a decision restate the decision in the message body, not "see plan X." `git log` is permanent; plans move. | Capture rationale at the diff ↔ Don't infer rationale later from the diff | `commit-messages-as-capture.md` |
| 5. Session Discipline | Implementation sessions anchored to GitHub Issues (or equivalent durable tracker). Captures at session boundaries. | Anchor context to a durable home ↔ Don't rebuild context from chat history | `session-discipline.md` |
| 6. Proof Bundle | Completion claims require a structured proof file written from live repo state — `git diff`, test output, reachability artefact — before any task is marked done. | Verify once against repo reality ↔ Don't recall plan-state as repo-state | `proof-bundle.md` |
| 7. Adversarial Verification | The verifier must be a fresh-context session loaded only with slice artefacts; the implementer cannot certify its own work. Returns PASS / FAIL / BLOCKED. | Certify once with independent context ↔ Don't let optimism authenticate itself | `adversarial-verification.md` |
| 8. Requirements Fidelity | The spec is not an axiom: every need is verified (29148 quality), validated (human sense-check), and traced (need → AC → test → proof) so a need can't drop silently between intake and spec. | Verify the requirement once, before delivery ↔ Don't re-discover a dropped need downstream | `requirements-fidelity.md` |
| 9. Design Fidelity | Design stays human-owned, with judgement calibrated to each choice's stakes (reversibility × blast-radius); Type-1 choices carry a recorded human decision the model can't self-authorise. | Decide the design once, with the right human attention ↔ Don't re-litigate or reverse-engineer the choice later | `design-fidelity.md` |
| 10. Customer Journey Validation | Critical end-to-end journeys are a ratified, version-controlled artefact, re-walked against real boundaries; a journey walked over a mocked boundary proves nothing. | Capture the journey once and re-walk it for real ↔ Don't ship a stale or mock-faked end-to-end path | `customer-journey-validation.md` |
| 11. Process-Global Mutation Guard | Any change mutating process-global state (working directory, environment, worktree/branch selection) must guarantee restore, assert the target before acting, and show a reachability artefact proving the guard. Especially load-bearing under parallel/multi-worktree execution. | Mutate scoped and restore once ↔ Don't let an unrestored mutation silently corrupt the next unit of work | `process-global-mutation.md` |

## Release Mode harness

Rules 6 through 11 are operationalised through a comprehensive harness:

- **Mechanical gates** — Baton specifies each gate (what it checks, that it fails closed); the reference implementation, the open `sworn` binary, runs them: the trace gate (RTM + EARS + sniff-test; `sworn trace`), the coverage gate (AC → test mapping; `sworn coverage`), the design-conformance gate (colours + architecture rules; `sworn designaudit`), the mock-boundary gate (undeclared mock boundaries), the regression gate (post-merge full suite; `sworn regress`), the proof-bundle verification gate (proof-bundle structure; `sworn verify`), and the board oracle (state-machine resolution from the git refs; `sworn board`). Baton itself ships no binaries.
- **LLM check types (6)** — `spec-ambiguity` (planner), `design-review` (captain), `ac-satisfaction` (implementer + verifier), `security-review` (implementer + verifier), `semantic-coverage` (verifier), `maintainability-review` (implementer + verifier). Deterministic (temp=0), structured prompts, structured JSON output, fail-closed; run via the LLM-check gate (`sworn llm-check --check <name>`).
- **Slice folder template** — `release-mode-template/{spec,proof}.json` + `status.json` + `journal.md`, plus release-level `board.json` + `intake.md`. The records (spec, proof, status, board) are **emitted** and validated against their schemas, then rendered to Markdown (`index.md`, the rendered proof) for human review; the prose files (`journal.md`, `intake.md`) stay hand-authored Markdown. Copy to `docs/release/YYYY-MM-DD-<theme>/<slice-id>/`.
- **Role prompts** — `role-prompts/planner.md`, `role-prompts/implementer.md`, `role-prompts/verifier.md`, and `role-prompts/captain.md`. Paste verbatim into agent sessions. The verifier prompt must always go into a *fresh* context window. The captain prompt runs `/design-review` (Rule 9), surfacing design pins for the Coach to acknowledge or push back on.
- **JSON Schemas** — record schemas validating the emitted loop records (`board-v1`, `spec-v1`, `proof-v1`, `slice-status-v1`, `journeys-v1`, `attestations-v1`), plus config schemas (`architecture-rules-v1`, `design-fidelity-v1`, `design-allowlist-v1`, `architecture-overrides-v1`). Hosted at `baton.sawy3r.net/schemas/`.
- **Brainstorm patterns** — `brainstorm-patterns.md`. Five visual patterns (Option Matrix, Decision Card, Scope-Ceiling Bar, Dependency Graph, Deferral Card) that make planning decisions discrete, visible, and capturable. Mandatory during Phase 2/3 of the planner role.
- **Track mode** — `track-mode.md`. The model for *safe parallelism*: slices are grouped into touchpoint-disjoint **tracks**, each implemented sequentially in its own `git worktree` on a `track/<release>/<track-id>` branch. Tracks run in parallel; `/merge-track` lands a finished track on the release branch and `/merge-release` lands the release on the version branch. Supersedes the earlier one-worktree-per-release model.
- **Architecture rules** — `architecture.json`. Project-level architectural rules with four check types (grep, touchpoints, diff-size, external). Declares canonical architectural documents (data model, API contracts, component hierarchy, ADRs, design tokens). Per-release overrides via `architecture-overrides.json`. Per-slice escape hatches via `design-allowlist.json`.

The harness is intentional: mechanical gates catch missing structure, LLM checks catch content failures, and adversarial verification ensures no role certifies its own work. It is the floor needed to make Rules 6 through 11 enforceable rather than advisory.

## Provenance

These rules emerged from a v0.5.0 release audit on the a single-developer SaaS monorepo (May 2026). The audit traced ~45 punch-list items across multiple "completed" plans where primitives shipped as dark code, schemas were silently deferred, and substantial analysis lived only in conversation. The rules are deliberately drafted as the **minimal intervention** that would have prevented those specific failures — not as a complete engineering methodology.

The full provenance and case study lives in `docs/captures/2026-05-13-v0.5.0-audit-handoff.md` in the source monorepo. That capture is itself an artefact of Rule 3 (Capture Discipline) — written specifically because the analysis was about to be lost to a `/clear`.

## Independence and path conventions

- **No plugin dependencies.** This package is plain markdown plus one optional shell script. It does not require any specific Claude Code plugin or any external tool beyond what your team already uses (git, GitHub or equivalent issue tracker, your test framework). The role prompts and brainstorm patterns bind to Claude Code's native `AskUserQuestion` as an implementation note; adopters on other tools render the same patterns inline.
- **Tool-agnostic.** Rules apply equally to Claude Code, Cursor, Copilot CLI, Aider, Gemini CLI, and human-only teams.
- **Path conventions are illustrative.** Examples use `docs/captures/`, `docs/plans/`, `docs/baton/` because those are concise and conventional. Adapt to your project's existing structure — the rules apply whether you keep captures in `docs/captures/`, `docs/decisions/`, `notes/sessions/`, or any other durable location. What matters is that the location is durable, version-controlled, and discoverable; the name is not load-bearing.
- **Provenance citations are historical.** When rule docs cite a specific the source project path like `docs/captures/2026-05-13-...`, that is a citation to the source monorepo's case study, not a prescriptive path for adopters.

## Complementary, not a replacement

These rules are **baseline engineering rigour**, not a full methodology. They are designed to coexist with richer frameworks (your team's existing process, formal SDLC) — not replace them.

A specific lesson learned at the source project (the source case study): introducing a sophisticated agent methodology (in this case an earlier internal harness) added valuable capabilities — brainstorming, plan-writing, code-review skills — but loosened baseline rigour around reachability, capture, and decision-tracking. The audit that produced these rules found that the loosening was the larger contributor to project churn than any individual rule violation.

The intent of Baton is to restore that baseline rigour as a **floor that survives plugin churn, methodology evolution, and team rotation**. If your team uses a sophisticated agent methodology, run this package underneath it. If your team prefers minimal tooling, this is enough on its own.

## How to adopt

See `INSTALL.md`. Quickest path:

1. Copy the contents of `AGENTS-fragment.md` into your project's `AGENTS.md` (or equivalent — `CLAUDE.md`, `GEMINI.md`, repo-level instructions).
2. Add the optional user-level rules from `CLAUDE-md-user-level.md` to your `~/.claude/CLAUDE.md` (or equivalent) if you want them to apply across all your projects.
3. Seed your per-project agent memory with the rule provenance from the relevant rule files.

Adoption can be partial. The Reachability Gate alone catches the largest single class of failures; everything else is incremental.

## What this is NOT

- Not a coding style guide (formatting, naming conventions, language preferences).
- Not a security policy (use your existing one).
- Not a project-management methodology (Scrum/Kanban/etc.).
- Not opinionated about specific AI tools — the rules apply equally to Claude Code, Cursor, Copilot, Aider, and human-only teams.
- Not a substitute for review — these rules reduce the bug burden of review by catching common AI-assist failure modes upstream.
- Not a replacement for richer methodologies. See "Complementary, not a replacement" above.
- Not dependent on any plugin. Plain markdown, copyable into any project.

## Versioning

This rule-set follows semver against the *content* — rules, role prompts, templates, and patterns. Breaking changes (rewordings that change adoption behaviour, removed rules, renamed role contracts) bump major. New rules or new roles bump minor. New templates, new brainstorm patterns, clarifications, and examples — anything that augments existing rules or roles without changing their contract — bump patch. The current package version lives on the [Releases page](https://github.com/sawy3r/baton/releases); the historical evolution of the rules themselves is in `RULES-HISTORY.md`.

## License & contribution

Designed to be copied. Attribution appreciated but not required. If you adapt the rules and find new failure modes worth codifying, contributions back to the source repo are welcome.
