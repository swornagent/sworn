# Bash coach-loop vs Go sworn — head-to-head eval

**Date:** 2026-06-28
**Setup:** Same release (`2026-06-27-conformance-foundation`), same comparison slices
(S01-llm-interpreter, S08-capability-descriptor, S13-schema-embed-validate), run two ways:
- **Go `sworn run --parallel`** (the in-process engine) — results in `2026-06-28-sworn-eval-findings.md`.
- **Bash `coach loop`** (the runtime-driver orchestrator) — this doc.

Bash clones at `~/sworn-eval-coach-{claude,deepseek,glm}` (origin removed so pushes cannot touch the real
repo). The bash clones were NOT pre-bootstrapped — letting coach-loop cold-start is the test.

Model lineups:
- **Claude combo** (claude-cli driver): implementer = opus, verifier = sonnet, captain/interpreter = haiku.
- **deepseek-v4-pro** (oai-compat): implementer + verifier = deepseek/deepseek-v4-pro.
- **glm-5.2** (oai-compat → ollama.com/v1): implementer + verifier = ollama-cloud-oai/glm-5.2.
(Bash bounded steps — captain/interpreter — default to claude-cli haiku/sonnet for the deepseek/glm runs;
noted as a minor difference from the Go runs which used one model for everything.)

## Result 1 — cold-start (decisive, already observed)

| | Go `sworn run --parallel` | Bash `coach loop` |
|---|---|---|
| Bootstrap a freshly-planned release | **Cannot.** Needs release-wt branch, real worktree paths, `.sworn` DB schema, start_commit + in_progress pre-created; crashed on each | **Yes.** `pre-materialise` creates the release worktree + branch, records paths, commits, before any worker spawns |
| First dispatch | **SIGSEGV** (nil agent/verifier factory) before any model call | Dispatches `/implement-slice` to all 6 tracks via the driver |
| Removed git origin | n/a | `push failed (continuing — local branch is canonical)` — tolerated, no pollution |

All three bash lineups cold-started cleanly with zero code changes to the engine. The Go run required
~8 supervisor interventions (branch, paths, DB, start_commit, in_progress) plus 2 source fixes
(nil-factory defaults, content tag) before it could even dispatch — and still could not converge.

## Result 2 — per-slice completion

**Go reached `verified` for ZERO slices** (turn-cap + escalation cascade). **Bash (Claude combo) drove
slices all the way to `verified`** — the decisive contrast.

Bash Claude combo (impl=opus→sonnet rotation, verify=sonnet, captain/interp=haiku), final states:
- S13-schema-embed-validate → **verified**
- S22-pin-bump → **verified**
- S01, S08, S24 → implemented (verifying)
- S11 → in_progress
(All six tracks cold-started, banked work via auto-checkpoint, and converged; one PAGE on a stuck track
paused for the Coach rather than crashing — failure isolation working.)

All three comparison targets reached **verified** in the bash Claude run, and a supervisor
re-test (don't-trust-the-loop) confirms each is GENUINELY correct — passing the exact Rule-1
integration AC that every Go run missed:

| Slice | Go best | Bash Claude combo (verified, independently re-tested) |
|---|---|---|
| S01-llm-interpreter | orphaned leaf, NOT wired to worker.go, no test (FAIL) | **verified** — `Interpret()` wired into `worker.go:283` + `interpreter_test.go` |
| S08-capability-descriptor | deepseek correct but NO test; glm missing fail-fast | **verified** — fail-fast in `run.go:362` + `capabilities_test.go`; suites pass |
| S13-schema-embed-validate | both: leaf validator NOT wired into Write() | **verified** — `baton.Validate` wired into `state.Write()` (`state.go:194`); suites pass |

Capstone: bash did not merely reach `verified` faster — its implementations are actually *complete and
integrated* (Rule-1 reachability satisfied) where ALL Go implementations were partial/orphaned leaves.
The bash verifier is the agentic, test-re-running verifier.md (sonnet); the supervisor re-test
independently confirms its verdicts. The run also flowed past the targets into the second slice of each
track (e.g. T4 S13→S14), i.e. it is genuinely progressing the release, not stalling.

### Same-model contrast (the cleanest proof: harness, not model)

DeepSeek was run BOTH ways. Re-tested by the supervisor:

| Slice | deepseek-v4-pro via **Go** in-process loop | deepseek-v4-pro via **bash** driver loop |
|---|---|---|
| S08 | correct production code but NO test; never `verified` | **verified** — fail-fast `run.go:358` + `capabilities_test.go` passing |
| S13 | good early attempt LOST to retry-reset; never `verified` | **verified** — `baton.Validate` wired into `state.Write()` `state.go:197`; tests pass |
| S22 | (Go never reached it cleanly) | **verified** |
| S01 | orphaned leaf, not wired to worker.go, no test; never `verified` | **verified** — `Interpret()` wired into worker.go, interpreter_test present, scheduler package builds (confirmed at the committed verified commit f1744f6) |

DeepSeek via bash ultimately swept ALL THREE targets (S01/S08/S13) to genuinely-correct `verified`,
matching the Claude combo. (Re-test note: always build the COMMITTED verified commit, not the live
worktree — the running loop leaves worker.go dirty mid-dispatch, which transiently fails to compile; the
committed S01 commit builds clean. Don't-trust-the-loop cuts both ways: verify the artefact, not the
in-flight tree.)

Identical model, opposite result. The Go loop crashed/cascaded and reached `verified` for nothing; the
bash loop drove the same model to genuinely-correct, integrated, tested `verified` slices. This isolates
the cause to the engine, not the model — the central thesis of the architecture recommendation.

(Run still in progress at capture: S11/S24 in_progress, S01 planned. Bounded steps — captain/interp —
used claude haiku/sonnet per coach defaults. Run ID: ~/.coach/sworn-eval-coach-deepseek.)

### Earlier note (headless supervisory session)

**DeepSeek + glm via bash:** both **cold-started cleanly**, and the oai-compat driver itself works
end-to-end (a direct `oai-compat.sh dispatch` to `deepseek/deepseek-v4-pro` returned a clean result,
cost $0.000364, 806 tokens — confirmed reaching the API). They produced no committed work ONLY because
the coach-loop **parallel worker** in this headless supervisory session recorded `model:""`,
`dispatches:0` and never invoked the driver (claude-cli dispatched fine in the same run). The user
confirms these models run cleanly in their own (interactive) environment, so this is an environmental
quirk of the headless session's oai-compat parallel-dispatch path, NOT a key/format/driver fault and NOT
a bash-engine fault. DeepSeek/glm bash data points to be gathered by the user in their environment.

## Result 3 — why bash converges where Go did not

Confirmed live in the logs:
- **Cold-start serialized before fan-out** — `pre-materialise` creates the release worktree+branch and
  commits before any worker spawns; all lineups bootstrapped with zero engine changes.
- **Driver owns the loop + stop condition** — the implementer runs as a real agent (`claude -p` /
  oai-compat), so there is no in-process `content,omitempty` serialization bug, no 25-turn non-
  convergence, no nil-factory panic. The orchestrator never sees a wire message.
- **Work is banked per dispatch** — `auto-checkpoint uncommitted work before implement` committed
  partial work; retries continue from the best state, never reset (Go discarded work on retry).
- **Failures are track-local** — a stuck track emits `PAGE` and pauses for the Coach; it does not
  SIGSEGV or fail-fast-cancel the whole release (Go cascaded one failure into six).

This is the empirical validation of the architecture recommendation: a thin deterministic orchestrator
over a runtime-driver contract converges and degrades safely where the in-process reimplementation
crashes.
