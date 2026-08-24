# In-engine learning: preflight admission + capability/outcome graph — 2026-08-25

Spec drafted from the overnight 2026-08-23-unattended-operability run
(revisions r1–r17). That run delivered S1–S6 but cost seventeen
relaunches and six roster changes, and **every intervention was made by
a human operator**: each park was diagnosed by hand, written into
operator memory + GitHub issues + uncommitted engine patches. The
engine re-hit each failure class blind and would re-hit it again on the
next run. This spec closes that gap — it promotes the operator's
cross-run learning into engine-consulted structure.

## The finding, stated precisely

sworn's learning is durable on ONE axis and absent on the other:

- **Within a run** (solved): receipts adopt by ancestry, refusal paths
  carry into the next try, interrupted effects reconcile from committed
  records. The engine genuinely does not rediscover mistakes inside a
  dispatch lineage.
- **Across runs** (open): the cross-run learning layer is a human with
  a memory file. The engine captures outcomes durably (R2 telemetry)
  but **consumes none of them at decision time**. The data moat is
  recorded and unread.

The frontier is therefore: **turn the execution journal into a
persistent, retrieval-at-admission capability/failure memory** — and do
it with the memory-discipline the project already ratified (structural
facts hard-gate; stochastic facts stay soft priors, never gates).

## Design axis: structural (deterministic) vs stochastic (probabilistic)

Every fact the engine could learn falls in one of two classes, and they
get opposite treatment. Conflating them is the trap.

- **Structural facts** are deterministic and probe-discoverable: "codex
  has no Go toolchain", "this credential is expired", "balance is
  negative", "projected context exceeds this window". These become
  **hard admission gates with named refusal codes**, checked by cheap
  preflight probes BEFORE a dispatch burns.
- **Stochastic facts** are probabilistic and context-scoped: "grok
  windows on big-context implementer slices", "gemini reads greedily".
  These become **soft routing priors** with recency + confidence
  weighting — they bias selection, they never blacklist. (ADR-0013:
  eval is a soft prior, never a gate. Same rule here.)

Phase 1 (structural) is high-leverage and low-risk — build it first.
Phase 3 (stochastic) is the actual "learning" and needs care.

## Phase 1 — preflight admission probes (structural hard gates)

One new seam: before a role×adapter dispatch is admitted, run the cheap
probes that would have caught the night's parks. Each failure is a
NAMED refusal at admission, not a 20-to-47-minute sunk dispatch. This
unifies four already-filed issues into one preflight registry.

- **S1 credential + balance preflight** (extends #221 / R3-S1): the
  existing expiry check gains (a) a liveness/identity probe for
  CLI-native adapters (tonight: claude OAuth staleness, then the
  server-side 2.1.234 deprecation, both surfaced as opaque transport
  errors), and (b) a provider balance/quota probe (tonight: deepseek at
  -$0.02; the raw /user/balance and 429-body curls the operator ran by
  hand). Refuse CREDENTIAL_STALE / PROVIDER_EXHAUSTED_HARD up front.
- **S2 toolchain-capability preflight** (#238): refuse a role×adapter
  assignment that cannot execute the contract's declared checks. Codex
  native mounts no /usr, so `go test` cannot run in its closure —
  discovered 20 minutes into a dispatch tonight, refusable in one probe.
  Named code UNMET_CHECK_CAPABILITY at MANIFEST admission. (Also mount
  the toolchain into the native closure, or route native shell through
  the MCP executor — but the admission refusal is the safety net.)
- **S3 context-window admission — CORRECTLY SPLIT** (#236): only the
  OPENING context (inputs + injected workspace context) vs the
  adapter's declared window is deterministic, and only that part is a
  hard admission gate. Conversation GROWTH is model-behavior-dependent
  (gemini read 2.5MB greedily where deepseek did the same work in a
  128K window via range reads) — so growth belongs to (a) a RUNTIME
  input-growth guard that steers/parks with a named reason as the
  conversation approaches the window (mid-dispatch, not admission),
  and (b) the Phase-3 graph learning per-model read behavior as a soft
  routing prior. The overnight cliff was hit on gemini (1M, 47 min
  sunk) and grok (~490K observed, twice) — same mechanism, two
  providers; admission alone would NOT have prevented these, the
  runtime guard would have bounded them. Declared window sizes come
  from adapter config; observed rejection sizes are fallback evidence,
  not primary truth.
- **S4 CLI-pin liveness** (#220): a dispatch-time probe that the pinned
  native binary still transacts with its provider, distinct from
  cert-time certification. Tonight the claude 2.1.234 pin was certified
  and dead simultaneously — the provider deprecated it server-side. A
  semver-range admission policy (#220) plus a live-transaction probe
  turns this from a 40-minute archaeology into a named refusal.

Acceptance shape: a fixture manifest assigning an incapable role×adapter
is refused at admission with the specific named code and zero dispatch
burn; each probe has a unit test and a bounded timeout; probes are
side-effect-free and their results are journaled.

## Phase 2 — refusal taxonomy + captured evidence (the training signal)

The learning graph is only as good as the signal feeding it, and
tonight three distinct failure classes were all flattened to opaque
codes with no captured evidence:
- PROVIDER_TRANSPORT_FAILED for {OAuth stale, CLI deprecated, adapter
  hang} — three different causes, one opaque code.
- NATIVE_SURFACE_INVALID names none of its ~8 checks and captures
  nothing of the rejected request (#237).
- PROVIDER_REQUEST_REJECTED carried the provider's own message
  ("Insufficient Balance", "input token count exceeds 1048576") but the
  engine discarded it (#227 same class).

Changes:
- **Typed provider-error taxonomy** (the backlogged provider-error-
  taxonomy item): every driver-boundary refusal carries a typed
  Kind that distinguishes cause, and the retry/park policy consumes the
  Kind. Hard-exhaustion never retries into a window; a dead pin never
  paces.
- **Bounded request-head capture on refusal** (#237, R2-S6 precedent):
  each refusal persists a bounded head of the offending request +
  the named check that failed. Tonight's diagnoses required reading
  validator source and reasoning backwards; the head makes them a query.
- **Surface the provider message once per dispatch** (#227): the
  provider's own string ("monthly spending cap", "Insufficient
  Balance") reaches the operator/journal at least once instead of being
  cleared.

Without Phase 2, Phase 3 learns from mush. This is the prerequisite.

## Phase 3 — the capability/outcome graph (the durable learning layer)

The new substrate. A per-project, accumulating graph derived from the
journal (R2's data, finally consumed):

- **Nodes**: `(adapter/profile, model, role, work-shape)`. The
  work-shape descriptor is the feature key — role, touched-package set,
  contract check-set, projected read volume, expected duration band. It
  must be stable enough to generalize across slices (a big-context
  implementer slice on internal/runtime is the same shape whether it's
  S5 or S7).
- **Edges**: observed outcomes — success, failure-Kind (from Phase 2),
  context-size-at-failure, duration, cost (S6 landed provider-reported
  cost capture, so this is now real) — with **recency + confidence
  weighting** so one bad draw never permanently condemns a node. Grok
  captained and verified six slices flawlessly AND windowed twice; the
  graph must hold both without blacklisting grok.
- **Population**: a projector over the journal (same pattern as the
  cockpit projector), run-side, incremental. No new capture — R2
  already records everything; this reads it.
- **Per-fact-class lifetimes (staleness)**: learned facts expire on
  wildly different clocks and the graph must carry that. "codex-native
  mounts no toolchain" is durable until the closure changes; "deepseek
  balance negative" expires on a top-up; "claude 2.1.234 deprecated" is
  scoped to a pin version and dies with the next re-pin. A fact carries
  its invalidation condition (config digest, pin version, a re-probe
  interval) — stale facts re-verify by probe rather than being trusted
  forever. The literature flags exactly this as unsolved (knowledge that
  expires when the tooling changes underneath it); the invalidation-
  condition design is sworn's answer.
- **Provenance on every fact**: each learned fact points at its journal
  evidence (run id, effect id, observation digest) — auditable and
  invalidatable, never free-floating. Same discipline as
  [[feedback_decided_claims_need_provenance]]: an unsourced "decided"
  survives exactly in unfalsified territory, and an unsourced learned
  fact would rot the same way.

## Phase 4 — admission consults the graph (soft-prior routing)

At role×model selection the scheduler reads the graph as a SOFT prior:
- Structural facts (Phase 1) are already hard gates — inadmissible
  assignments never reach here.
- Stochastic facts bias the choice: "this work-shape windows on grok →
  prefer a larger-window model for the implementer seat", "this adapter
  reads greedily → pair it with the read-economics guard tuned tighter".
  Never a gate; always a weighted preference over the admissible set.
- This is ADR-0013 (capability-based selection) extended from a static
  declaration to a live-outcome-fed prior. The capability-policy stays
  the hard constraint; the graph sharpens the soft ranking underneath.

Worked example — the whole night in one admission check: the operator
tried five implementer engines to find the one that fit the night's
conditions (deepseek dry, ox protocol-incompatible, gemini window,
sonnet proxy-wall, grok). A populated graph does that search in one
consultation: given work-shape = big-context implementer on
internal/runtime, rank the admissible engines by their observed outcome
on that shape, skip the structurally-inadmissible, and open on the best
draw instead of the first.

## Adjacent: slice-dependency extraction (graph-engineering, sequenced later)

Separate from learning, but the same "graph" theme and worth capturing
— with an honesty correction from review: dependency here is NOT just
data-flow, it is **edit-surface overlap**. The overnight's slices
(S3/S4/S5/S7) all edit the same core files (turn_recovery.go,
dispatch.go, service.go); parallel tracks over overlapping touchpoints
buy merge conflicts, not throughput. So serializing this release was
probably RIGHT, and the earlier framing ("serialization cost the
overnight") overclaimed — the overnight was lost to provider weather,
which parallelism would have multiplied, not dodged. The durable idea
survives in corrected form: the plan-authoring surface (#234) should
**derive the slice DAG from contract touchpoints — overlapping
touchpoints force serialization, disjoint ones permit parallelism** —
turning "one track or N?" from a default into a derived, checkable
decision instead of a vibe.

CAVEAT that sequences this AFTER survivability: parallelism multiplies
provider exposure (N parallel tracks = N concurrent draws on the same
rate limits) and the operator can only hand-carry one revision at a
time. Parallelism is a throughput win you turn on AFTER the substrate
stops eating runs. Phases 1–4 make the substrate survivable; this comes
after.

## Adjacent: verify-node topology (multi-verifier fan-out)

The verify step is today a single agentic pass — one model, one
context, one PASS/FAIL. The ratified roster doctrine already flags the
verifier as the weak point (Rule 7), and the run history shows the two
real failure modes: **declared-not-executed** (the #218 repin verifier
passed by reading; the holes were only findable by executing e2e —
3rd recorded instance) and **single-point-of-judgment** (r15 had grok
verifying grok's own implementation; r16 had a verifier that could not
execute at all). Fan the verify node into a small subgraph when stakes
warrant:

- **Adversarial verification**: N independent skeptics each prompted to
  REFUTE the candidate; a finding survives only if a majority cannot
  refute it. Kills plausible-but-wrong PASSes.
- **Perspective-diverse lenses**: distinct verifier nodes for
  correctness / scope / does-it-EXECUTE — the execution lens exists
  precisely because declared-not-executed is the recurring class.
- **Cross-model diversity where possible**: verifiers from the same
  family share blind spots; diversity across models is the point — but
  it re-imports provider weather, so it is a stakes-gated option, not a
  default.

Honesty notes: (1) cost — verification already runs ~30–50 min/slice;
lenses multiply cost unless run in parallel, so this is gated to
high-stakes slices (assembly, migration, security-adjacent) or
low-confidence verdicts, not universal. (2) The Goodhart claim in the
graph-engineering discourse is mis-attributed: loop-vs-graph SHAPE does
not mitigate Goodhart — **independent, diverse verification does**.
This section is the actual Goodhart mitigation; topology alone is not.
(3) Tie to Phase 3: a verdict DISTRIBUTION (agreement, dissent,
per-lens outcomes) is a far richer training signal for the outcome
graph than a binary PASS/FAIL — multi-verifier slices feed the
learning layer disproportionately.

## Adjacent: reduce the failure surface at source (prevention beats learning)

Complement to the whole learning apparatus, raised repeatedly across
the overnight and worth stating as a principle: **the cheapest failure
class to learn-to-avoid is the one you eliminate entirely by making the
worker interface admit what models actually do.** Learning routes
around walls; this removes walls:

- `Read` gains `offset`/`limit` and a batched `{"paths": [...]}` form —
  models already reach for all three spellings (#188 documents
  limit/offset sent unprompted; ox-alpha's dup-key batching was a
  batching INTENT with no legal syntax; #236 measured the whole-file
  read tax at 93% of a blown 1M window). R4-S7 amendment.
- **Tell the worker its environment**: nothing names the toolchain path
  (/usr/local/go, off-PATH — every model re-derives the export by
  habit), the .git mask, or the read budget. A short environment-facts
  block in the worker context deletes a whole class of rediscovery —
  the same "don't rediscover" goal as the graph, achieved by
  disclosure instead of memory.

Each such fix shrinks what Phases 1–4 have to learn. Prevention first,
then learning for what cannot be prevented.

## Explicit non-goal (the over-automation boundary)

The graph hardens ROUTING and ADMISSION. It does NOT make judgment
calls: not "$100 xAI vs subscription", not "is this escalation a real
contract gap", not roster strategy. The ratified prior holds — the loop
structurally cannot self-notice cross-cutting gaps
([[feedback_rules_capture_not_omniscience]]); those stay in the
human/coach layer. Aim the graph at deterministic wall-avoidance and
outcome-informed ranking; keep the judgment where it belongs. Over-
learning (brittle heuristics from thin data) is the failure mode —
recency + confidence weighting and the structural/stochastic split are
the guardrails.

## Sequencing

1. **Phase 1 (preflight probes)** — highest leverage, lowest risk, all
   structural; unifies #221/#236/#238/#220. Honest counterfactual for
   the overnight: prevents two parks outright (balance, toolchain),
   catches a pin deprecation at the next dispatch instead of six burned
   tries later, and turns window rejections into named refusals plus a
   bounded runtime guard — it does not prevent growth-driven window
   hits, only bounds them. Ship first, standalone.
2. **Phase 2 (taxonomy + capture)** — the training signal; unifies #237
   observability half + #227 + provider-error-taxonomy. Prerequisite
   for Phase 3.
3. **Phase 3 (the graph)** — the durable substrate; consumes R2 data.
4. **Phase 4 (soft-prior routing)** — extends ADR-0013 with live
   outcomes.
5. **Reduce-at-source worker-surface fixes** — ride R4-S7; they shrink
   what the graph must learn, so they land before or beside Phase 3.
6. **Verify-node fan-out** — stakes-gated, after the substrate is
   stable enough to afford it; its verdict distributions then feed
   Phase 3.
7. **Slice-DAG extraction** — after survivability, with #234;
   touchpoint overlap forces serialization, disjoint permits parallel.

Phases 1–2 are pure hardening and could ship as the next release with no
new conceptual risk. Phases 3–4 are the actual in-engine learning and
want the literature (below) to inform the memory/retrieval structure
before contracts are authored.

## Literature grounding (primary sources, verified 2026-08-25)

A commissioned review pulled and id-verified the primary sources; the
full bibliography lives in the session record, key anchors here. The
strategic headline: **the field has converged on "the durable object is
an external memory, the agent is disposable" — and the exact variant
sworn needs (failure-specific, provenance-carrying, staleness-aware,
retrieved at ADMISSION) is the acknowledged open problem, not solved
work.** Sworn would be building into the gap, not reimplementing.

Directly load-bearing for Phase 3's design:
- **CBR for LLM agents** (review, arXiv 2504.06943): the
  Retrieve–Reuse–Revise–**Retain** loop is the theoretical backbone;
  Phase 3 is a CBR case bank keyed by work-shape.
- **Memento** (arXiv 2508.16153): the closest concrete instantiation —
  a non-parametric case bank of (state, action, outcome), retrieved by
  a selection policy at decision time, no fine-tuning. Validates the
  no-weights, journal-derived approach.
- **Agentic Context Engineering / ACE** (arXiv 2510.04618, ICLR 2026):
  evolving "playbook" via **structured incremental delta updates** —
  its "context collapse" result (a summarizer erodes the specific
  hard-won lesson) is the formal version of why learned facts must be
  delta-updated with provenance, never re-summarized wholesale.
- **Generative Agents** (arXiv 2304.03442): recency × relevance ×
  importance retrieval scoring — the default ranker for Phase 3
  retrieval, with the caveat (open problem) that it is a heuristic.
- **Where LLM Agents Fail** (arXiv 2509.25370): AgentErrorTaxonomy —
  a reusable schema to sanity-check Phase 2's failure-Kind taxonomy
  against.
- **ExpeL** (2308.10144), **Voyager** (2305.16291), **AWM**
  (2409.07429): the persist-and-retrieve lineage for rules, skills, and
  workflows respectively — capability-side precedents. **Reflexion**
  (2303.11366) is explicitly the WRONG pattern to copy: within-episode
  only, discarded on reset — the gap sworn fills.
- Orchestration side: Anthropic's multi-agent research system +
  dynamic workflows posts (orchestrator holds plan, workers
  disposable); scheduler-theoretic DAG framing (2604.11378, position
  paper, no empirical validation — framing only); Halo batched agent
  DAGs (2509.02121); RouteLLM (2406.18665) as the outcome-derived
  routing seed for Phase 4. LangGraph's checkpointer model is the
  production reference for resumability — which sworn's journal already
  exceeds in durability semantics.

Open problems the literature names, which are exactly this spec's
frontier: failure-specific admission-time memory (unbuilt anywhere),
staleness/expiry of learned facts when tooling changes underneath
(unsolved — the per-fact invalidation-condition design above is
sworn's answer), retrieval precision (heuristic rankers), and
provenance/trust of retained knowledge. One id (2606.31270,
learning-from-failure for computer-use) was found but not
independently verified — treat as a search lead, not a citation.
