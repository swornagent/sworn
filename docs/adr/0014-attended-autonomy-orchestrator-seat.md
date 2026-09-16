# ADR-0014: Attended autonomy - an orchestrator seat above an attested, provider-agnostic team runtime

- Status: Accepted (Brad, 2026-09-15)
- Supersedes the "fully unattended delivery" framing of the 2026-08-23
  unattended-operability line and the survivability-first release queue
  that followed it.

## Context

Every release Sworn has delivered was driven by an orchestrator sitting
above the engine: a Claude session reading the board, diagnosing the
journal, retrying, answering attention turns, revising plans and filing
gaps. The Go loop has never run a real release unattended. The last two
releases (2026-09-05 preserve-work, 2026-09-11 phased-evidence) spent most
of their candidate rounds on survivability machinery: try budgets,
identical-failure and exhaustion parks, session continuity, wait-and-resume.
Each replaced one judgement call with a rule, and each rule then met its
next uncovered case (sworn#296, #300, #304-#310). Seven runs and six plan
revisions produced one verified slice.

Claude Code workflows already provide the orchestration ergonomics (fan-out,
pipeline, verify, resume) with none of that overhead. What they cannot
provide: a choice of provider and model per role, evidence that a check
actually ran on the exact tree, receipts, exactly-once effects, sandboxed
workers, cross-provider telemetry, or human authority boundaries that hold
whichever model is driving. Sworn already has all of those working.

## Decision

1. **Sworn is a provider-agnostic, attested team runtime that an
   orchestrator drives.** The orchestrator seat holds the judgement:
   retries, flake calls, plan revisions, attention answers, when to stop.
   Sworn holds what the seat cannot do itself: per-role provider and model
   choice (ADR-0013), sealed candidates, host checks that provably ran,
   receipts, exactly-once effects, sandboxed workers, telemetry, and the
   human authority boundary (release trigger stays human-scoped).
2. **Running fully unattended is a non-goal.** Parks and attention turns
   are the engine's escalation interface, not defects to engineer away.
   Anything that decides on the engine's behalf is suspect by default.
3. **The first seat (the Manager) is a Claude Code skill over the cockpit MCP surface**
   (`sworn_status`, `sworn_control` retry/cancel/pause/resume/grant/takeover,
   `sworn_attentions` and `sworn_answer_attention`, `sworn_approve`,
   `sworn_start`). It formalises the operator work that delivered every
   release to date. A workflow-script seat and a built-in seat may follow;
   they are the same interface driven differently.
4. **The survivability machinery is kept and demoted to advisory.** Parks
   keep firing as typed escalations; automatic retry chains become opt-in
   behind a manifest knob that defaults off, so the seat decides retries.
5. **Provider choice stays explicit and recorded, never a hidden fallback.**
   The value is auditable routing, not a router.

## The seat's context is a projection, never a transcript

Durable state lives in Sworn (journal, board, receipts, operator log). The
seat is stateless between ticks by design. Each tick loads a bounded set:
plan digest, `sworn_status`, the pinned work with its typed cause and
detail, open attention text, and the seat's own short decision log.

- Sworn owes the seat **"why" as typed facts**: park cause, refusal code
  and detail, check output excerpt, the real Lead receipt. Every time
  the seat must open a raw journal to learn why, that is an engine
  legibility gap and is filed as one (sworn#306, #307, #310 are this class).
- **Deep dives are delegated.** A subagent reads the journal and returns a
  conclusion; the seat's own context stays one slice wide.
- **Loop state lives in the script or the record, not in the model.**

## The seat learns; it does not have amnesia

Learning is three layers, each with a durable home and a consumer. The
2026-08-25 in-engine learning discipline applies throughout: structural
facts hard-gate, stochastic facts stay soft priors, and no learned fact
bypasses human authority.

1. **Decision journal (raw material).** Every orchestrator decision is a
   Sworn record: the situation (typed cause, work, evidence digests), the
   decision, the rationale, and later the outcome. Today this is the ops
   home operator-log.md plus the operator's memory file; it becomes a
   first-class record so any fresh seat resumes from it, and so outcomes
   can be joined back to decisions.
2. **Policy distillation (rules the seat loads).** The principal-approvable
   decisions catalogue becomes a versioned, project-scoped policy file:
   "in situation X, decide Y, because Z, evidence: decisions a,b,c". The
   seat loads it every tick. New entries are proposed from the decision
   journal and ratified by a human. Structural entries (a re-sealed
   proposal with unchanged substance is approvable; an anchor substitute
   is adjudicated in scope) are what the catalogue already holds.
3. **Telemetry priors (what the engine consumes at admission).** The
   in-engine learning spec's Phase 1 preflight registry (credential,
   balance, toolchain and context-window probes as named admission
   refusals) and, later, Phase 3 stochastic routing priors from
   per-dispatch outcomes. These are the only learned facts the engine
   itself acts on, and only as gates for structural facts and priors for
   stochastic ones.

## Vocabulary

Ratified 2026-09-16, with the principle that names are descriptive and
instantly recognisable; generic is fine. Protocol, the former public protocol
repo, is retired and decommissioned, so this is a Sworn-internal rename.

| Seat | Was | Does |
|---|---|---|
| Principal | Principal | The accountable human: approves plans, answers Type-1 escalations, ratifies policy. |
| Director | (new) | Portfolio seat over several releases: priorities, budgets, results, escalations. Delegated authority only. |
| Manager | (new) | Release seat: runs one release, sets the roster, routine calls within policy, keeps the decision journal. Delegated authority only. |
| Lead | Lead | Per-slice design authority in the run: reviews designs, adjudicates scope, unblocks. `lead_review` becomes `lead_review`, `lead_plan_review` becomes `lead_plan_review`. |
| Planner, Implementer, Verifier | same | Unchanged. |
| Scout | (later) | Learning advisor over outcomes: recommends rosters and priors, never selects. |

"Orchestrator" describes the Manager and Director tier; it is not a role.
The package formerly named after the retired protocol becomes
`internal/protocol` (plans, contracts, receipts, state and record actions),
and action names follow (`protocol.merge`, `protocol.install`). "Authority"
was the first choice and was rejected because it is already a local
variable name throughout the engine and would shadow the package.
ADRs 0001 to 0010 stay as
written and read through this table. Journals recorded under the old
vocabulary are not migrated. The rename is one focused pass ahead of
Track A, done by hand.

## Consequences and queue

Demoted: sworn#292 session continuity (a cost optimisation, not a
survivability requirement); the bounded wait-and-resume ask of #310; every
further autopilot guard.

Promoted, in order:

- **Track 0, vocabulary.** The rename above.
- **Track A, seat.** A `/sworn-orchestrate` skill: tick loop over
  `sworn_status`, decision policy loaded from the catalogue file, typed
  actions through `sworn_control` and attention answers, decision journal
  writes. Deliverable: one release driven end to end by the skill with no
  hand operator work.
- **Track B, legibility.** sworn#294 live worker stream and per-turn
  journaling on native lanes; the driver half of #314 (record the CLI's
  error result as the cause); a typed "why" audit of every park and refusal
  the seat currently has to reconstruct from the journal.
- **Track C, advisory autopilot.** Manifest knob for automatic retry
  chains, default off; parks unchanged; exhaustion becomes "no retry
  policy chosen" rather than "budget spent".
- **Track D, learning.** Decision journal record and read surface; the
  catalogue as a policy file with a proposal path; preflight registry
  Phase 1 from the 2026-08-25 spec.
- **Track E, release 2026-09-11-phased-evidence.** Promote the verified S1
  (seal-time gates, sworn#300/#301) alone; re-scope S2-S4 under this ADR
  rather than resuming run r8.

Not changed: the Protocol protocol boundary (ADR-0010), capability-based
selection (ADR-0013), the human-scoped release trigger, and every
attestation seam.
