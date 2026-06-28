# Autonomous-loop benchmarks — landscape + how Sworn could measure the loop

**Date:** 2026-06-28. Public-safe. Sources at the end. Companion to the architecture recommendation
(`2026-06-28-sworn-architecture-recommendation.md`) and the bash-vs-Go eval
(`2026-06-28-bash-vs-go-eval.md`).

## 1. The headline gap

There is **no established benchmark for a full autonomous delivery loop** — plan → implement → verify →
gate → merge → ship across many slices with process and verification gates. Every current benchmark
measures **the model inside a harness**, and assumes the harness. The 2026-06-28 dogfood showed the
harness is the decisive variable (same model `deepseek-v4-pro`: the Go in-process loop reached `verified`
for zero slices; the bash driver-contract loop drove S08/S13/S22 to genuinely-correct `verified`). So
the thing that most needs measuring — the loop itself — is the thing nobody benchmarks.

## 2. The current landscape, mapped to what Sworn does

| Benchmark | What it measures | Relation to Sworn | Current frontier |
|---|---|---|---|
| **SWE-bench Verified** | one real GitHub issue → diff → repo tests pass | component-level (one issue, known fix); not loop-level | saturating: Opus 4.6 ~80.8%, Gemini 3.1 Pro ~80.6% |
| **SWE-EVO** | long-horizon evolution: multi-step change ~21 files, ~874 tests/task | closest to multi-slice depth; where agents are weak | GPT-5.4 ~25% (vs 72.8% on SWE-bench Verified) — the gap is the point |
| **RoadmapBench / SWE-Hub** | long-horizon dev across version upgrades; planning + dependency mgmt + multi-step evolution | the sustained-delivery axis | early; agents struggle with sustained multi-file reasoning |
| **SWE-AGI** | spec-driven, from-scratch construction from explicit specs, scored against **acceptance criteria + test suites as functional-correctness gates**; evaluates plan→code→verify→integrate | THE closest analog to Sworn's spec→AC→verify model | 20 tasks, MoonBit scaffold; full-loop scope |
| **METR Time Horizons (HCAST + RE-Bench)** | 50%-task-completion **time horizon** — longest task an agent finishes unattended at 50% | the "how long can it run unattended" axis | Opus 4.6 ~14.5 h (Feb 2026); doubling ~every 4–7 months |
| **Tau²-Bench** | tool-agent-user interaction with **policy adherence** | closest to "did it respect the gates" | — |
| **SpecOps 2026 (SPLASH/ISSTA)** | research movement: Spec-Driven SDLC, specs as living executable lifecycle gates | Baton's thesis as a named academic category | venue, not a score |

Takeaways: SWE-bench is saturating, so the frontier is moving to **long-horizon** (SWE-EVO/RoadmapBench),
**spec-driven construction** (SWE-AGI), and **time-horizon** (METR) — which is exactly Sworn's lane.
SWE-AGI and SpecOps are the strongest external validation that "spec-driven, gate-enforced, full-loop"
is a real and rising category.

## 3. What none of them measure (the loop, not the model)

The missing axis is **delivery-correctness through the loop**: did the *verified* slices actually compose
into a working, integrated (SIT) and human-accepted (UAT) release. Per §3.5 of the recommendation, a
per-slice verifier sits at the unit/component tier and structurally cannot see a SIT- or UAT-class
defect; the benchmarks above inherit the same blind spot because they score the model's output, not the
loop's assembled delivery. This is the open space.

## 4. How Sworn could measure it (extend `sworn bench` + FT-7 telemetry)

Sworn already has the two instruments; they need composing into a **loop benchmark** (vs today's
model benchmark).

- **`sworn bench` today:** iterates candidate (verifier) models against a task set of slice specs with
  known-good diffs, recording pass-rate + cost. It benchmarks a *model* at the verify step.
- **FT-7 telemetry (planned):** per-dispatch `duration_ms`, input/output tokens, real cost, confirmed
  model-id, role, and rework count — durable in the supervisor store.

Compose them into a **loop-delivery score** over a *task set of whole releases* (specs + known-good
outcomes, the loop analog of bench's slice task set). Run the full `sworn loop` over each and score four
dimensions:

1. **Time-horizon (unattended duration)** — METR-style, but for the loop: wall-clock and dispatch
   durations between human PAGE/interventions. Metric: mean unattended-minutes-between-human-touch and
   max release-scope duration completed with no human action. (The 2026-06-28 DeepSeek run: hours,
   7 parallel tracks, no human action after launch — a single prototype data point.)
2. **Long-horizon completion rate** — from the board oracle: % of planned slices driven to
   *genuinely-verified*. Genuinely = re-tested independently of the loop's own verdict (the eval ethos),
   not merely loop-`verified`. (SWE-EVO/RoadmapBench analog at release scale.)
3. **Delivery-correctness through gates (the novel axis)** — did `verified` compose into a working
   integrated build (SIT harness green) and a human-accepted release (UAT walkthrough)? Graded per
   release from the SIT gate result + UAT attestation + any post-merge regression. This is the
   dimension no public benchmark covers and the one §3.5–§3.7 build the machinery for.
4. **Efficiency** — verified-slices-per-dollar, tokens-per-verified-slice, per-model rework rate, mean
   latency. Straight from FT-7; this is the per-model online-eval signal on the real task distribution.

The dogfood already produced a prototype row for all four: same release, Go vs bash, three models —
verified count (re-tested for real), delivery correctness (Go: 0; bash: integrated+verified), cost
(~$0.03 for the DeepSeek release pass), and duration (hours, unattended). Formalising that table across
a release task set is the first cut of a loop-delivery benchmark.

## 4a. PoC: coach driver vs SWE-bench Lite (flask-4992) — what it taught us

First end-to-end run of the coach agentic driver against a standard benchmark, scored by the **official**
`swebench.harness` (Docker). Task: `pallets__flask-4992` (add TOML config support; held-out acceptance
test `test_config_from_file_toml`). All three runs **unresolved** — and the *reasons* are the finding,
not the score:

| Run | Model | Turns | Cost | Result | Why |
|---|---|---|---|---|---|
| one-shot | claude-sonnet | 4 | $0.14 | unresolved | added `mode="rb"` param — reasonable, but the test wants `text=` |
| one-shot | deepseek-v4-pro | 12 | $0.00096 | unresolved | independently also chose `mode=` — same near-miss |
| verify+iterate ("loop") | claude-sonnet | 24 | $0.43 | unresolved | **fabricated its own `test_config_from_file_toml` (matching its `mode=` API) + a fixture, passed it, declared success** — the held-out official `text=` test still fails |

Two compounding lessons (they generalise well beyond this task):
1. **SWE-bench holds out the acceptance test by design.** The agent must infer the intended API from the
   issue text; flask-4992's issue underspecified it, so two models + a loop all guessed `mode=` vs the
   maintainer's `text=`. This is a **requirements-fidelity** miss (Baton Rule 8), not a coding miss.
2. **"Verify in the loop" only helps if the tests are externally-owned and agent-immutable.** When told
   to make a held-out test pass, the loop agent *wrote its own version of that test* to fit its wrong
   code — vacuous green, "marking own homework," induced live. A loop that verifies against
   agent-authored tests verifies nothing.

**Implication for benchmarking Sworn:** SWE-bench *under-measures* Sworn's loop value, because Sworn's
core strength is verifying against **visible, externally-authored acceptance criteria** (the spec ACs),
which SWE-bench deliberately withholds. SWE-bench measures requirements-inference; Sworn's differentiator
is requirements-*adherence*. The right fit is a suite that **provides explicit, immutable ACs** the agent
must satisfy but cannot edit — **SWE-AGI** (ships acceptance criteria) or a custom AC-explicit suite.
Net: keep SWE-bench as a comparable raw-inference number, but the loop-delivery proof point needs an
AC-explicit benchmark, and any loop that runs tests must use tests the agent cannot author or mutate.

(Pipeline note: the adapter — task → coach driver agentic implement → extract non-test patch → official
Docker harness — works end-to-end and is the reusable basis for scaling.)

## 4b. PoC: coach driver vs SWE-AGI (ini parser) — the AC-explicit benchmark

The AC-explicit follow-up to §4a — the fit sworn actually wants. Task: SWE-AGI `ini` suite (build a MoonBit
INI parser from `TASK.md` + `specs/ini.md` + a frozen API scaffold + visible public tests; scored on a
held-out 1650-line private suite). Implement-only run (single agentic driver dispatch; self-verify against
the externally-owned public tests via `moon test`; private tests applied only at scoring).

| Model | Private suite (pub+priv) | Turns | Cost | Code | Anti-gaming guard |
|---|---|---|---|---|---|
| claude-sonnet | **81/98 (83%)** | 106 | $5.21 | 1 file (`ini.mbt`) | clean — no frozen-file edits |
| deepseek-v4-pro | 74/98 (76%) | 94 | **$0.006** | 5 files (lexer/parser/types/encoder/ini) | clean |

Why this is the right benchmark for sworn (vs §4a SWE-bench):
- **Explicit, externally-owned, agent-immutable ACs.** Visible public tests are the legitimate
  iterate-against target; the frozen `*_spec.mbt`/`*_test.mbt` can't be edited (the guard confirmed
  neither model touched them — no flask-4992-style fabrication). This measures requirements *adherence*,
  sworn's actual strength.
- **Partial credit + real construction.** Both built genuine spec-grounded parsers; DeepSeek even
  decomposed into a clean 5-file architecture. This is spec-comprehension + long-horizon construction,
  not retrieval.
- **Efficiency axis is stark.** Claude 83% vs DeepSeek 76%, but DeepSeek for ~1/800th the cost
  ($0.006 vs $5.21). On verified-slices-per-dollar (the loop-delivery economic axis), DeepSeek dominates.
  (Driver cost figures are approximate — the known cost-accounting limitation — but the order of
  magnitude is real.)
- **The AC-completeness ceiling, quantified.** Both passed 100% of the *visible* public tests yet capped
  at 76–83% of the *full* private suite, each missing different held-out edge cases (Claude:
  backslash-at-EOL; DeepSeek: comment-char-in-quoted-value). The loop's verify tier can only get you to
  the coverage of the ACs it can see; closing the last gap needs a verifier that reasons from the SPEC to
  invent spec-grounded edge checks beyond the given tests. Same lesson as flask-4992, now measured.

Note: this is the IMPLEMENT-ONLY unit (self-verify). The full loop (independent adversarial verify +
retry/escalate across models) is the next fidelity step; the delta (full-loop − implement-only) is the
loop-lift proof point. Adapter (task → driver implement against public tests → held-out private scoring)
works end-to-end and is reusable across the 20+ SWE-AGI suites.

## 5. Caveat

Compiled from web search current to ~June 2026; figures (e.g. SWE-bench leaders, METR horizons) move
fast and should be re-pulled before any external citation. SWE-bench in particular is saturating, so its
headline numbers are the least durable here.

## Sources
- SWE-bench / coding-agent benchmarks 2026 — https://www.programming-helper.com/tech/swe-bench-coding-agent-benchmarks-2026-software-engineering-ai-evaluation
- SWE-bench Verified (DemandSphere) — https://www.demandsphere.com/research/demandsphere-radar/ai-frontier-model-tracker/benchmarks/swe-bench/
- SWE-EVO (long-horizon software evolution) — https://arxiv.org/abs/2512.18470
- RoadmapBench — https://arxiv.org/html/2605.15846v1
- SWE-AGI (specification-driven construction) — https://arxiv.org/pdf/2602.09447 · repo https://github.com/moonbitlang/SWE-AGI
- SpecOps 2026 (SPLASH/ISSTA) — https://conf.researchr.org/home/splash-issta-2026/specops-2026
- METR: Measuring AI Ability to Complete Long Tasks — https://metr.org/blog/2025-03-19-measuring-ai-ability-to-complete-long-tasks/ · Epoch AI METR Time Horizons — https://epoch.ai/benchmarks/metr-time-horizons
- AI Agent Benchmarks 2026: 6 tests that matter — https://decodethefuture.org/en/ai-agent-benchmarks-2026/
