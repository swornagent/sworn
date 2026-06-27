# Sworn architecture evaluation and recommendation

**Date:** 2026-06-28
**Inputs:** the three-model dogfood (`2026-06-28-sworn-eval-findings.md`), the bash coach-loop
learnings (`2026-06-28-bash-coachloop-learnings.md`), the conformance audit
(`2026-06-27-baton-conformance-audit.md`), and the surface-seam model (`2026-06-27-surface-seam.md`).
**Lens (requested):** best-practice software engineering and architecture, evaluated for four
properties: performance, memory management, resilience, architectural elegance.
**Audience:** Brad (decision-maker). This is a recommendation, not a decision; promote to an ADR if accepted.

---

## 1. Thesis

Sworn's deterministic core is genuinely good. The damage is concentrated in one architectural choice:
**Sworn reimplemented, in-process, two things that should be external contracts.**

1. **The agentic loop and every provider's wire format** (`internal/agent` + the `internal/model`
   provider matrix). This is where the universal `content,omitempty` serialization bug, the 25-turn
   non-convergence, the Responses-API `input[].output` bug, the nil-factory SIGSEGVs, and the
   keyless/Anthropic capability gaps all live. One small struct tag took down every provider at once
   because the core owns the wire format.
2. **The slice lifecycle bootstrap** that the slash-command layer (Driver 1) actually owns. The
   autonomous engine assumed the `release-wt` branch, real worktree paths, the `.sworn` DB schema, and
   `start_commit`+`in_progress` already existed, and crashed when they did not.

The bash coach-loop avoids this entire class of failure with one design move: **a thin, deterministic
orchestrator over a runtime-driver contract.** The orchestrator never touches a provider's JSON and
never owns the tool loop; a driver (`claude-cli`, `codex`, a hardened `oai-compat`) owns the loop and
returns one normalized result line. The orchestrator's job is routing, durable state, scheduling, and
supervision, which is exactly what Go does well.

**North star:** Sworn should be a thin, durable-state, deterministic orchestrator over a stable
**Driver contract**. It should never reimplement a provider's wire protocol or the agent tool-loop in
its core. "One engine, three surfaces" (slash / MCP / `sworn run`) is right; the missing dual principle
is **"one orchestrator, N drivers."**

---

## 2. Root-cause diagnosis: the missing seam

```
            TODAY (Go)                          RECOMMENDED (bash already does this)
  ┌─────────────────────────────┐      ┌──────────────────────────────────────────┐
  │ orchestrator (router, sched)│      │ orchestrator (router, sched, durable state)│
  │ + internal/agent (tool loop)│      └───────────────────┬──────────────────────┘
  │ + internal/model (wire fmt) │              Driver contract (Dispatch → result line)
  │   per provider × capability │      ┌──────────┬─────────┬──────────┬───────────┐
  └─────────────────────────────┘      │claude-cli│ codex   │oai-compat│ ollama    │
   one bug in the wire format =         │(delegate │(delegate│(1 hardened│(delegate) │
   every provider breaks                │ to agent)│to agent)│ in-proc) │           │
                                        └──────────┴─────────┴──────────┴───────────┘
                                         a provider quirk is contained in one driver
```

The audit already flagged the symptom ("agentic Chat thin; only OAI+OpenAIResponses implement Chat;
capability checked at runtime not resolution"). The dogfood proved the consequence: the in-process loop
could not complete a single slice on any model. The fix is not to harden each of nine providers; it is
to stop owning the wire format in the core.

---

## 3. Evaluation by property

### 3.1 Resilience — the dominant gap (today: poor; target: the core competency)

A delivery engine's primary quality is resilience: it must make progress, or stop safely, under partial
failure. Sworn currently fails this on every axis the dogfood touched.

| Failure mode (observed) | Why it happens | Bash technique | Recommendation |
|---|---|---|---|
| Cold-start crash | Bootstrap assumed, racy across workers | `ensure_release_worktree()` runs **once before any worker spawns**, commits, fails loud | Serialize bootstrap before fan-out; fail-closed on every missing path (Rule 11 target assertion); never let a worker create shared state |
| Nil-factory SIGSEGV | In-process factory left nil on one code path | `driver_path()` validates the driver executable at the boundary; missing → error, never nil | Driver factory returns `(Driver, error)`; no nil call paths; default to an always-available driver |
| Lost work on retry | Retry resets the worktree to start_commit | `commit_worktree_wip()` checkpoints **before every dispatch** | Auto-WIP-commit each dispatch; retries continue from best state; verifier reads history |
| One failure kills the release | `failCancel()` cancels the whole phase | Per-track isolation; a failure PAGEs, siblings continue | Track failures are local; PAGE the Coach; never cross-cancel healthy tracks |
| Non-OpenAI run cascade-fails | Hardcoded `openai/*` escalation models | Per-role model lists from `COACH_IMPL_MODELS` etc. | Per-role/model escalation from config; consume the `Error{Kind}` taxonomy (terminal → halt, transient → retry); never hardcode a provider |
| Loop never converges | 25-turn cap, model emits no terminal text | Exit on **"no tool calls pending"**, turn cap is only a circuit-breaker; force one summary pass | Driver owns the stop condition; exit-on-no-tools; force-summary on empty text |

**Principle:** model the loop as a supervised state machine where **every transition is durable**
(git commit + `status.json`) and **every external call may fail without crashing the process**. A
crash, a kill, or a provider 5xx should be recoverable by re-reading committed state. This is FT-1
(orchestration) and FT-2 (error taxonomy) and is the highest-value work in the release.

### 3.2 Performance — optimise completed-slices-per-token, not turns-per-second (today: misleading)

Raw dispatch speed is not Sworn's bottleneck; **wasted work** is. The dogfood burned real tokens to
produce almost nothing: 25-turn non-convergence produced no result, retry-reset discarded completed
files, and fail-fast cancelled in-flight sibling work. Throughput collapsed not because Go is slow but
because the loop cannot bank progress.

- In-process goroutine fan-out is cheaper per dispatch than bash's subprocess+CLI model, but that
  advantage is moot while the loop cannot finish a slice. Sub-process driver overhead (spawn + IPC) is
  negligible against model latency (seconds to minutes per turn), so **delegating the loop to a driver
  costs ~nothing in wall-clock and removes the entire serialization bug class.**
- The real performance levers are the resilience fixes: exit-on-no-tools (stop paying for turns that
  add nothing), work-preservation (never re-pay for completed work), and failure isolation (don't throw
  away healthy in-flight work).
- **Recommendation:** define the performance KPI as **verified-slices per 1k tokens / per dollar /
  per wall-clock hour**, and instrument it (FT-7 telemetry: `duration_ms`, real token split, real cost,
  confirmed model-id, rework count). The dogfood could not even produce this number because the loop
  never reached `verified` and cost reads 0. Measuring the right thing is itself a prerequisite.

### 3.3 Memory management — bound the transcript, isolate the cache (today: unbounded by parallelism)

- **In-process transcript growth.** `internal/agent` appends every turn's assistant + tool messages to
  a growing history, and tool outputs include file contents. With 6 parallel tracks each holding a
  growing transcript, resident memory scales with `parallelism × transcript size`. There is a tool-
  output cap, but the full history is retained for the dispatch and only released when the goroutine
  returns. Long slices with many large file reads are the worst case.
- **Worktree cache pollution.** Tool exec wrote a Go module cache (`go/pkg/mod`, deepseek) and a build
  cache (`.cache/go-build`, glm, ~1047 files) **into the worktree**. That is disk, not RAM, but it
  bloats the worktree, the diff, and any snapshot, and required `chmod -R u+w` to clean.
- **Subprocess drivers get memory hygiene for free.** A driver process owns its transcript and releases
  it on exit; the orchestrator retains only small normalized result lines. Memory is naturally bounded
  per dispatch regardless of parallelism.
- **Recommendation:** (a) if delegating to subprocess drivers, transcript memory is bounded by
  construction; (b) for any in-process driver, stream/trim history and release it promptly after the
  dispatch; (c) set `GOCACHE`/`GOMODCACHE`/`HOME` for tool exec to a path **outside** the worktree
  (inherit the repo's `.env`, as the bash driver does), so caches never enter the diff.

### 3.4 Architectural elegance — the Driver contract is the elegant seam (today: smeared)

- Sworn smears two concerns that want to be separate. The **orchestration plane** (router, scheduler,
  oracle, state machine) is deterministic, pure, testable, and is the part the audit praises. The
  **agent plane** (tool loop + provider wire format) is messy, provider-specific, and fast-moving. They
  live in the same process and the same failure domain today, so a wire-format bug crashes the
  orchestrator.
- The bash design is more elegant precisely because it draws the boundary at the process edge: a
  `Dispatch(spec, worktree) -> {status, result_text, cost, ...}` contract. Providers' quirks are
  contained in one driver; adding a model is "does a driver exist," not "extend a 9×N capability
  matrix." This also dissolves the FT-2 capability-descriptor problem: capability is "a driver is
  registered for this model," surfaced at resolution, not discovered mid-run.
- The surface-seam doc already articulates "three drivers (surfaces), one core." The structural
  counterpart is **"one orchestrator, N runtime drivers."** Adopting the same `baton/runtime-drivers.md`
  contract the bash loop uses would also keep Baton (protocol) and Sworn (impl) coherent.
- **Recommendation:** introduce a `Driver` interface at the process boundary. Make `internal/agent` +
  `internal/model` an **implementation detail behind it** (one hardened `oai-compat`-style in-process
  driver, with the S27 content fix), and add a **subprocess driver** that delegates to a real agent CLI
  (`claude-cli`/`codex`) for correctness and memory isolation. Default to the subprocess driver; the
  orchestrator must never see a `ChatMessage`.

---

### 3.5 The verification gap that shipped Sworn DOA (the recursive lesson)

**Sworn was built by the bash coach-loop using the FULL agentic, fresh-context, test-re-running
verifier — and it still shipped DOA.** This is the most important lesson, and it corrects an earlier
over-claim that the bash verifier's "mechanical teeth" made it sound. A rigorous per-slice verifier is
necessary but not sufficient.

Why a full verifier passed a DOA system: per-slice verification checks each slice against its own spec
ACs and tests, but those tests **mocked the boundary** — the slices that built the parallel loop were
exercised with an injected fake `NewAgent`/`NewVerifier` and mock models, so their unit tests went green
without ever running the real `sworn loop --parallel` from a cold board against a real provider. Green
leaves did not compose into a working binary. Nothing in the loop ran the assembled affordance
end-to-end, so the nil-factory SIGSEGV, the `content,omitempty` serialization wall, and the cold-start
gaps were all invisible to a verifier that only ever saw mocked, leaf-level slices.

This is precisely **Baton Rule 1** (the first failing test must render through the integration point that
owns the affordance, not the leaf) and **Rule 10** (a journey walked over a mocked boundary proves
nothing) — failing at the meta level. The recursion is the point: **Sworn, the tool built to enforce
reachability and no-mock journeys, was produced by a loop that did not enforce them on Sworn itself, and
shipped DOA as the direct result.**

Implication for the recommendation: the fix is two-pronged, and the architecture half alone is not
enough.
- **Architecture** (sections 1-3): delegate the loop to a driver so the whole class of in-process
  wire/loop bugs cannot exist.
- **Gates**: make per-slice ACs demand a *real* reachability artefact (the integration point executes,
  not a mocked leaf), and wire the **Rule-10 no-mock end-to-end journey** gate so a release cannot
  certify until `sworn loop` actually boots and runs a slice against real infra from a cold board. The
  "keyless-full-loop" journey this release was meant to declare IS that gate; it was not enforced on the
  release that defines it. A full verifier plus mocked ACs equals confident green over a DOA build.

## 4. Prioritised recommendations (mapped to release tracks + properties)

| # | Recommendation | Property | Track | Effort |
|---|---|---|---|---|
| 1 | Serialize cold-start bootstrap before fan-out; fail-closed target assertions | Resilience | FT-1 | M |
| 2 | Auto-WIP-commit before every dispatch; never reset worktree on retry | Resilience/Perf | FT-1 | S |
| 3 | Track-local failure + PAGE; remove phase-wide `failCancel` cascade | Resilience | FT-1 | S |
| 4 | **Define the `Driver` contract; move agent-loop/wire-format behind it; default to a subprocess agent driver** | Elegance/Resilience/Memory | FT-2 | L |
| 5 | Per-role/model escalation from config; consume `Error{Kind}` (terminal→halt) | Resilience | FT-2 | M |
| 6 | Exit-on-no-tools stop condition; turn cap as circuit-breaker; force-summary | Perf/Resilience | FT-2/FT-3 | S |
| 7 | Tool-exec cache/env hygiene (GOCACHE/GOMODCACHE/HOME outside worktree) | Memory | FT-1 | S |
| 8 | Telemetry KPI = verified-slices per token/$/hour (duration, real cost, model-id, rework) | Perf | FT-7 | M |
| 9 | S27 already landed: nil-factory defaults + always-emit content (interim, keep until #4) | Resilience | T1/FT-2 | done |
| 10 | **Reachability ACs through the real integration point (no mocked-leaf green) + wire the Rule-10 no-mock end-to-end gate so a release can't certify until `sworn loop` boots and runs a slice against real infra** | Resilience/Correctness | FT-3 + Rule-10 | M | see §3.5 |
| 11 | Rename `sworn run` → `sworn loop` (loop-engineering terminology); `run` kept as deprecated alias | Elegance | — | done |

Recommendation #4 is the keystone: it subsumes most of the model-layer audit gaps and is the single
change that would have prevented the dogfood's headline failures. #1, #2, #3 are small and turn the loop
from "crashes the release on any hiccup" into "banks progress and stops safely."

---

## 5. Sequencing

1. **Stabilise (small, immediate):** #1, #2, #3, #7. These make the loop survivable without touching the
   model layer. After these, the existing in-process loop (with S27) can at least bank partial work and
   isolate failures.
2. **Re-seam (the keystone):** #4 + #5 + #6. Introduce the Driver contract, delegate the loop, retire
   the in-process provider matrix as the default. This is where elegance, resilience, and memory all
   improve together.
3. **Measure (continuous):** #8. Stand up the telemetry so the next eval produces real
   verified-slices-per-dollar numbers per model (the data moat).

---

## 6. Empirical validation available: run the same eval through the coach-loop

The bash coach-loop can run the **same release and the same models** end-to-end (see
`2026-06-28-bash-coachloop-learnings.md` Part B). Running it is the cleanest proof of this
recommendation: the bash loop should complete slices the Go loop could not, on the same models, because
it delegates the loop to a driver and banks work between dispatches. This directly tests the thesis
rather than arguing it. Proposed: run it on the same comparison set (S01/S08/S13) with deepseek-v4-pro
and glm-5.2, and compare completed-slices + tokens against the Go run. Awaiting go/no-go.
