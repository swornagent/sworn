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
- **S3 context-window preflight** (#236): estimate the dispatch's
  opening context (inputs + workspace + projected read budget for the
  work-shape) against the adapter's declared window; refuse or reroute
  before the provider rejects mid-endgame. Tonight this cliff was hit on
  gemini (1M, 47 min sunk) AND grok (~490K, twice) — same mechanism,
  two providers. Pairs with the read-economics guard below.
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

Separate from learning, but the same "graph" theme and worth capturing:
tonight T1-unattended ran S1–S8 as ONE serial track. If S6/S7/S8 have
no real data dependency (different seams), serializing them is the
false-sequentiality the graph-engineering discourse warns about, and it
cost the whole overnight. The plan-authoring surface (#234) could
**compute the real slice DAG from contract touchpoints and refuse to
serialize what has no dependency** — turning "one track or N?" from a
default into a derived, checkable decision.

CAVEAT that sequences this AFTER survivability: parallelism multiplies
provider exposure (N parallel tracks = N concurrent draws on the same
rate limits) and the operator can only hand-carry one revision at a
time. Parallelism is a throughput win you turn on AFTER the substrate
stops eating runs. Phases 1–4 make the substrate survivable; this comes
after.

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
   structural; unifies #221/#236/#238/#220; would have prevented ~4 of
   tonight's parks outright. Ship first, standalone.
2. **Phase 2 (taxonomy + capture)** — the training signal; unifies #237
   observability half + #227 + provider-error-taxonomy. Prerequisite
   for Phase 3.
3. **Phase 3 (the graph)** — the durable substrate; consumes R2 data.
4. **Phase 4 (soft-prior routing)** — extends ADR-0013 with live
   outcomes.
5. **Slice-DAG extraction** — after survivability, with #234.

Phases 1–2 are pure hardening and could ship as the next release with no
new conceptual risk. Phases 3–4 are the actual in-engine learning and
want the literature review (in flight) to inform the memory/retrieval
structure before contracts are authored.
