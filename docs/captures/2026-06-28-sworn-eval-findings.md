# Sworn three-model eval — findings

**Date:** 2026-06-28
**Context:** Three-model comparison build of release `2026-06-27-conformance-foundation` through
SwornAgent, supervised by a chat agent (external interpreter). Models: A=gpt-5.3-codex,
B=deepseek-v4-pro, C=glm-5.2 (ollama-cloud). This is also a dogfood: Sworn is building the
fixes it lacks, so it hits its own gaps. The loop's verdicts are NOT trusted; tests are
re-run independently by the supervisor.

**Comparison set (all three models build exactly these):**
- `S01-llm-interpreter` (T1, orchestration — hardest; new `interpreter.go` + `worker.go` wiring)
- `S08-capability-descriptor` (T2, model-layer — broad, 12 files; `Capability` type + per-driver methods + fail-fast resolution)
- `S13-schema-embed-validate` (T4, records; embed schema + validator + `state.Write()` validation)

Legend: `confirmed:<x>` = known-expected gap from the conformance audit. `NOVEL` = not previously catalogued.

---

## Setup / environment findings

- **confirmed (stale binary):** the `sworn` on PATH (`~/go/bin/sworn`) was `0.0.0-dev` predating the
  orchestration commands (`board`, `run --parallel`, `--release`). Had to `go install ./cmd/sworn`
  from the `release/v0.1.0` checkout. Version string stays `0.0.0-dev` (ldflags only on release build).
- Keys present in `~/.config/coach/env`: OPENAI_API_KEY, DEEPSEEK_API_KEY, OLLAMA_CLOUD_API_KEY (+others).
  ollama-cloud OpenAI-compat endpoint = `https://ollama.com/v1` (base var = `https://ollama.com`).
  No alias mismatch needed beyond exporting SWORN_OPENAI_API_KEY / SWORN_DEEPSEEK_API_KEY from natives.

## Cold-start / orchestration findings (the headline)

- **NOVEL — `sworn run --parallel` cannot cold-start a freshly-planned release.** On a clean clone
  whose slices are all `planned`, the autonomous engine (Driver 3) fails immediately. It assumes the
  release worktree + `release-wt/<name>` branch were already created by a prior `/implement-slice`
  (Driver 1) bootstrap. The engine is not self-sufficient for a cold release. This is the single
  biggest gap surfaced: Driver 3 depends on Driver-1 scaffolding that the board's own placeholders
  (`# set by first /implement-slice in this release`) document but the engine never performs.
  Error: `materialise release worktree: exit status 128 / fatal: invalid reference: release-wt/<name>`.
  Ref: `internal/run/parallel.go:134-145`.

- **NOVEL — YAML inline comments parsed as literal path values.** `extractReleaseWorktreePath`
  (`internal/run/parallel.go:299-303`) and `ParseTracks` (`internal/board/track.go:37`,
  regex `^\s+worktree_path\s*:\s*(.*)$`) both naively capture everything after the colon. The
  frontmatter placeholder `release_worktree_path: # set by first /implement-slice in this release`
  yields the *comment text* as the path — non-empty, so it passes the `== ""` guard
  (`parallel.go:124`) and is fed to `git worktree add`. (Note: `internal/board/index.go` HAS a
  `reConcatKey` detector for "key hidden after # comment", but it is a lint-path check, not applied
  on the run path.)

- **NOVEL — multi-clone isolation defeated by hardcoded `ProjectDir`.** `cmd/sworn/run.go:127`
  hardcodes `ProjectDir: "sworn"`. When a track's `worktree_path` is empty, the worker defaults the
  track worktree to `~/projects/sworn-worktrees/release-<release>-<trackID>`
  (`internal/scheduler/worker.go:153-154`) — identical for EVERY clone. The task's premise that
  "separate .git = no branch collision" holds for branches but NOT for worktree paths: a second
  clone's run sees `dirExists==true` and silently operates on the first clone's worktree
  (cross-contamination). Real isolation requires overriding each track's `worktree_path` to a
  clone-local path. Also a Rule-11 process-global concern (shared mutable path across runs).

- **NOVEL (display) — board `actionable` is hard-stuck `false`.** `internal/board/oracle.go:230`:
  the assignment `actionable = isActionable(s.State)` is fused onto the end of a `//` comment line
  (lost newline), so it is commented out; `actionable` never updates from its `false` initialiser.
  `sworn board --json` reports `actionable:false` for every slice including freshly-planned,
  ready-to-run ones. Display-only: the scheduler/router do NOT consume `oracle.Actionable`
  (they use `findFirstNonTerminal` + router state logic), so the run is not blocked by it. Same
  "lost-newline corruption" class flagged previously for index.md frontmatter.

- **NOVEL — supervisor DB dir not created (`.sworn/` missing).** `openDefaultDB` (`cmd/sworn/run.go:199-216`)
  sets `dbPath = <wd>/.sworn/sworn.db` but never `os.MkdirAll`s `.sworn/`. Because sqlite `sql.Open`
  is lazy, the failure surfaces late as `supervisor: begin tx: unable to open database file (14)` and
  EVERY track FAILs at supervisor-acquire before any work. The engine assumes `sworn init` (Driver 1)
  already created `.sworn/`. Workaround: `mkdir -p <clone>/.sworn` per clone.

- **NOVEL — supervisor schema never migrated in production (`no such table: tracks`).** The schema
  (`tracks`/`events`/`schema_version`) is created by `internal/db.Open` (`internal/db/db.go:53`, which
  also does `MkdirAll` — i.e. fixes Bug 5 too), but production `openDefaultDB` (`cmd/sworn/run.go:211`)
  uses raw `database/sql.Open`, bypassing migration. Tests use `internal/db.Open`; production does not.
  Root cause is shared with Bug 5: `openDefaultDB` reimplements DB-open instead of delegating to
  `internal/db.Open` (a one-line fix). Workaround: pre-create each clone's `.sworn/sworn.db` via the
  real `internal/db.Open`.

- **NOVEL — `RunSlice` requires `start_commit` pre-set; engine never sets it.** `RunSlice`
  (`internal/run/slice.go:119-122`) reads `start_commit` from status.json and hard-errors if empty.
  Planned specs have `start_commit: null`; `/implement-slice` (Driver 1) writes it on the
  `planned→in_progress` transition. The autonomous engine does not perform that transition, so it
  cannot run a slice that a human/Driver-1 hasn't already moved to `in_progress`. Same cold-start
  pattern. Workaround: set `start_commit` for all slices on the `release-wt` branch.
- **OBSERVED (by-design, harsh) — phase fail-fast.** Any single TrackFail calls `failCancel()`
  (`internal/run/parallel.go:223-225`), cancelling all sibling goroutines in the phase
  (`worktree materialisation failed: signal: killed`). One un-bootstrapped slice kills the whole
  release run. Defensible as fail-fast, but combined with the cold-start gaps it means a release
  with any single un-bootstrapped slice produces six FAILs and zero progress.

- **NOVEL (SEVERE, headline) — `sworn run --parallel` SIGSEGVs on the first slice dispatch.**
  `run.Run` (single-slice) defaults `opts.NewAgent = newAgentFromModel` when nil
  (`internal/run/run.go:107-108`), but `RunSlice` (the parallel path) never defaults it and calls
  `opts.NewAgent(...)` directly (`internal/run/slice.go:165`). The parallel `runSliceFn`
  (`cmd/sworn/run.go:113-120`) does not pass `NewAgent`, so it is nil → nil-pointer panic at the
  design-TL;DR step, BEFORE any model call. Conclusion: the autonomous `--parallel` loop has never
  worked from the real CLI; it only runs in tests, which inject a fake agent factory. The headline
  orchestration feature is dead on the cold path. **One-line fix** (mirror run.Run's default in
  RunSlice) applied to the shared engine + rebuilt, identically for all three clones, purely to let
  the eval proceed (does not bias the model comparison).

- **NOVEL — `NewVerifier` also nil in parallel mode (2nd SIGSEGV).** Same root cause as Bug 8: after
  fixing NewAgent, the run reached the verify step and panicked at `internal/run/slice.go:374`
  (`opts.NewVerifier(...)`). `run.Run` defaults both NewAgent and NewVerifier (run.go:107-111);
  RunSlice defaulted neither. Both one-line defaults applied.
- **NOVEL — engine cannot implement a `planned` slice (`implement: cannot run from state "planned"`).**
  `internal/implement/implement.go:52-78` only auto-transitions from `design_review`→`in_progress`
  (running the DoR gate); from `planned` it hard-errors. The router routes `planned→implement`, but
  nothing moves the slice to `design_review`/`in_progress` first. The `planned→in_progress` transition
  is Driver-1 (`/implement-slice`) bootstrap the engine skips. Workaround: set `state:in_progress`.
- **NOVEL — double-path join in RunSlice file ops.** `absSliceDir := filepath.Join(worktreeRoot,
  filepath.Dir(specPath))` (`internal/run/slice.go:147`) joins the worktree root with an already-absolute
  specPath, producing `/…/T4-records-as-json/home/brad/…/T4-records-as-json/…/design.md` (path doubled).
  Surfaced as `design: write design.md: … no such file or directory`. Affects design.md (and any other
  `absSliceDir`-derived path); `statusPath` is passed absolute and used directly so implement() still
  reads state. worker.go passes an absolute specPath where RunSlice expects a repo-relative one.
- **NOVEL — agentic implement loop broken across OpenAI-family drivers.** Multi-turn tool sessions
  fail at the provider: oai driver → `Invalid value for 'content': expected a string, got null`
  (assistant/tool message serialization, `internal/agent/agent.go:138-162` + `internal/model/oai.go`);
  openai-responses driver → `Missing required parameter: 'input[2].output'` (Responses API function-call
  echo). No model completed an implementation through this path. Matches the audit's "agentic Chat thin/
  buggy; only OAI+OpenAIResponses implement Chat, checked at runtime." This is a structural wall, not a
  per-model quirk — it blocks the whole implement→verify→merge loop.
- **NOVEL/confirm — `gpt-5.3-codex` via openai-responses returns HTTP 520** (Cloudflare edge error)
  on every call (design TL;DR + implement). Likely an invalid/unavailable model id or a responses-endpoint
  rejection. The implementer model produced zero output; the loop escalated to `openai/gpt-4o-mini` then
  `openai/gpt-4o` (which connect but hit the content-null protocol bug above). Confirms the audit's
  model-id concern. Escalation path itself works (router advanced 5/5 models).

- **NOVEL (ROOT CAUSE, universal) — `content,omitempty` breaks the multi-turn agent loop on every
  provider.** The shared request struct `internal/model/oai.go:81` tags `Content` as
  `json:"content,omitempty"`. On a tool-call turn where the model returns only `tool_calls` and no
  prose (`Content==""`), the field is dropped from the JSON. DeepSeek rejects
  `messages[N]: missing field content`; OpenAI rejects `content: expected a string, got null`. This is
  the single root cause behind ALL the per-driver agent-loop errors above. Importantly, deepseek
  reached **turns 3–7 doing real tool work** (reading spec, exploring code) before a tool-only turn
  triggered it — so the loop runs; it is killed by this one struct tag. **Fix** (`json:"content"`, always
  emit) applied to the shared engine + rebuilt, identically for all three clones. This is the minimal
  change without which zero implementation is possible (the model-comparison payoff is impossible).
  NOTE: the openai-responses driver (codex) has a SEPARATE Responses-API bug (`input[N].output`) and is
  also blocked by gpt-5.3-codex's HTTP 520; this fix does not rescue codex.
- **NOVEL — implementer tool-exec pollutes the worktree with a read-only Go module cache.** After a
  codex run, `~/sworn-eval-codex-wt/T7-.../go/pkg/mod/...` existed with read-only files (cleanup needed
  `chmod -R u+w`). The agent's tool execution ran `go` with a module cache resolving INTO the worktree.
  Side effects: worktree pollution + un-removable trees + the cache would be swept into the slice diff.

### Supervisor interventions to unblock (fair, identical across all three clones)
The autonomous engine could not cold-start; the eval required performing the bootstrap that
`/implement-slice` (Driver 1) does, plus minimal engine fixes. ALL interventions are identical across
the three clones and touch no slice spec/implementation, so they do not bias the model comparison.

**Scaffolding (per clone, no code change):**
1. `git branch release-wt/<release> release/v0.1.0`.
2. Rewrote `index.md` frontmatter placeholder comments → clone-isolated paths: `release_worktree_path`
   → `~/sworn-eval-<M>-wt/release`; each track `worktree_path` → `~/sworn-eval-<M>-wt/<trackID>`.
3. Pre-created `.sworn/sworn.db` via the real `internal/db.Open` (schema migration).
4. Set `start_commit` + `state:in_progress` + `owner:agent` on all 26 slice status.json (committed
   on the `release-wt` branch so materialised track worktrees inherit it).

**Engine fixes (minimal "keep the loop moving", applied to the shared `~/projects/sworn` source +
rebuilt once → same binary for all three runs):**
- A. `internal/run/slice.go`: default `opts.NewAgent`/`opts.NewVerifier` when nil (mirror run.Run) —
  fixes the two SIGSEGVs. Without this the parallel loop crashes before any model call.
- B. `internal/model/oai.go:81`: `content,omitempty` → `content` — fixes the universal agent-loop
  serialization wall. Without this no model can complete a multi-turn implement.
- NOT fixed (left as the release's actual work, characterised only): double-path join, design.md write,
  Responses-API `input[].output`, gpt-5.3-codex 520, board actionable, schema validation, etc.

---

## Per-model run findings

### B — deepseek-v4-pro (after content fix)
- Agent loop now SURVIVES multi-turn tool sessions (content fix B holds — no more serialization rejects).
- New failure mode: **`agent loop: turn cap (25) reached with no text response`** — the implementer
  makes 25 tool calls without converging to a terminal text answer. NOVEL finding: the 25-turn cap is
  too low for a real implementation slice (or the model loops without a stop signal). Slice fails →
  triage resolve_in_place retry. (cap ref: agent.go MaxTurns.)
- deepseek nonetheless wrote real, on-spec code before being cut off. One slice reached `verifying`
  (an attempt did converge to terminal text). Final preserved output (committed to track branches):
  - **S08** (T2): 13 code files — modified all 11 model drivers + `run.go` + new `registry.go`. Broad,
    on-touchpoint. Strongest deepseek output.
  - **S01** (T1): 2 files — new `internal/orchestrator/interpreter.go` + modified `internal/verify/verify.go`.
    Partial + DIVERGENT: no `interpreter_test.go` (spec-required), and it edited `verify.go` not the
    spec's `internal/scheduler/worker.go` integration point (Reachability-gate miss).
  - **S13** (T4): LOST — earlier attempt created `validator.go`/`validator_test.go`/`schemas/slice-status-v1.json`
    (good), but a later retry RESET the worktree and the final state has zero code. See retry-reset finding.
- **NOVEL — retries reset the worktree, discarding the best attempt.** S13 had a near-complete validator
  at 01:24; after resolve/escalate retries the final worktree had none. The loop preserves the LAST
  attempt, not the best — and a late attempt that dies early (context-canceled) wipes good prior work.
- **NOVEL — hardcoded `openai/*` escalation models cascade-fail a non-OpenAI run.** `DefaultEscalationModels`
  are `openai/gpt-4o-mini`/`gpt-4o`/… When the deepseek run exhausted its primary-model retry budget it
  escalated to `openai/gpt-4o-mini` → `RunSlice: create implementer agent: SWORN_OPENAI_API_KEY not set`
  → TrackFail. Combined with phase fail-fast, the first slice to exhaust retries cancels every sibling
  track (`context canceled` mid-dispatch) → entire release FAILs. A run with only a DeepSeek (or any
  non-OpenAI) key cannot survive a single primary-model retry exhaustion.

### A — codex (openai-responses/gpt-5.3-codex)
- Implementer model `gpt-5.3-codex` returns **HTTP 520** on every call (design TL;DR + implement) — no
  output produced. openai-responses driver also hits `Missing required parameter: 'input[2].output'`
  (Responses-API multi-turn echo bug; not addressed by content-fix B). Escalation to `openai/gpt-4o`/
  `gpt-4o-mini` connected but hit the content-null bug (pre-fix). **Net: codex produced zero code.**
  Re-running post-fix would still fail on the 520 + Responses-API bug, so codex was not re-run.

### C — glm-5.2 (openai/glm-5.2 via ollama.com/v1)
- Connects and runs via ollama-cloud (no auth/connectivity errors). Same agent loop → same 25-turn-cap
  failure mode as deepseek (content-fix B holds; no serialization errors). Slower than deepseek and
  produced LESS code per attempt. Same escalation cascade risk (escalation `openai/gpt-4o-mini` would
  route to ollama.com/v1 which lacks that model). NOVEL: glm's tool-exec polluted the worktree with a
  `.cache/go-build/` (1047 files) — different env path than deepseek's `go/pkg/mod`; both confirm
  tool-exec writes caches INTO the worktree (would be swept into the slice diff if not excluded).
- Stopped manually after a fair shot (it never reaches `verified`; turn-cap loops indefinitely).

## Per-slice three-way comparison

Independent verification by the supervisor (NOT trusting the loop). Test commands are the slice specs'
own reachability commands, re-run by hand against each model's preserved worktree output.

### S08-capability-descriptor (T2, model-layer)
- **codex:** no output (HTTP 520).
- **deepseek-v4-pro:** STRONG. Production code faithful to spec — `Capability` bitset + `CapabilityProvider`
  interface (`client.go`), `Unconfigured`→0, all 11 drivers given `Capabilities()`, `registry.go`
  `DriverCapabilities` map, and `ResolveImplementerModel` fail-fast with the exact spec error string
  ("driver %s does not support Chat — required for the implementer role"). `go build` clean; full
  `internal/model` + `internal/run` suites PASS (no regressions). GAP: no `capabilities_test.go` (the
  spec-required unit test), so the reachability command `go test … -run TestCapabilit` is vacuously green
  ("no tests to run"). Supervisor verdict: production PASS, **test-coverage FAIL** (would fail a strict
  Rule-1 verifier on the missing AC test, otherwise correct & complete).
- **glm-5.2:** INCOMPLETE. `Capability` type in `client.go` + `Capabilities()` on 7/11 drivers
  (anthropic, cli, client, oai, azure, bedrock, google). Builds clean. BUT **no `ResolveImplementerModel`
  fail-fast check** in run.go (the central user-outcome AC — "fail at startup with 'does not support
  Chat'" is absent), **no `registry.go`**, and 4 drivers (oci, ollama, env, openai_responses) lack the
  method. Supervisor verdict: **FAIL** (central AC unmet; partial mechanical coverage).
- **WINNER: deepseek** — decisively. Near-complete, correct, regression-free production code; only the
  unit test is missing. glm covered ~60% of the mechanical surface and missed the core fail-fast outcome.

### S01-llm-interpreter (T1, orchestration — hardest)
- **codex:** no output (HTTP 520).
- **deepseek-v4-pro:** PARTIAL + Reachability fail. Created `internal/orchestrator/interpreter.go` — a
  clean leaf (`Interpret(ctx, v model.Verifier, raw)`: nil→INCONCLUSIVE fail-closed, single bounded call,
  prefix parse, fail-closed default). BUT (a) signature diverges from spec (`(ctx, raw, sliceID, role)`),
  (b) **`internal/scheduler/worker.go` never calls `Interpret()`** — the integration point that owns the
  affordance is unwired (Baton Rule-1 violation: leaf built without the integration glue that was the
  required TDD red), (c) no `interpreter_test.go`. `go build` clean but `-run TestInterpreter` = no tests.
  Supervisor verdict: **FAIL** (orphaned leaf, not reachable, untested).
- **glm-5.2:** no output (S01 never produced code before the run was stopped; glm was slowest here).
- **WINNER: deepseek** — by default; the only model to produce any S01 artefact, though it too failed the
  Reachability gate (interpreter not wired into worker.go).

### S13-schema-embed-validate (T4, records)
- **codex:** no output (HTTP 520).
- **deepseek-v4-pro:** LOST to retry-reset. An early attempt produced a faithful `validator.go`
  (embed `schemas/*`, `ReadSchema`, structural `Validate` for slice-status-v1 enforcing the 4 required
  non-empty string fields — exactly spec option (b)), `validator_test.go`, and `schemas/slice-status-v1.json`.
  A later retry reset the worktree; final state has zero S13 code. Supervisor verdict: **no final artefact**
  (the loop discarded a near-correct implementation — a process failure, not a model failure).
- **glm-5.2:** PARTIAL + tested leaf, NOT integrated. Created `internal/baton/validator.go`,
  `validator_test.go`, `schemas/slice-status-v1.json`. `go build` clean; `go test ./internal/baton/...
  ./internal/state/...` PASSES (it wrote a real passing test — the embed-read AC test). BUT did NOT
  modify `internal/state/state.go` `Write()` to call the validator (the core integration AC) and left
  `example.com` in `internal/state/state_test.go` (AC violated). Same Rule-1 leaf-without-integration
  pattern. Supervisor verdict: **partial PASS** — best surviving S13 artefact, with a passing test, but
  the integration AC (validate-on-write) unmet.
- **WINNER: glm** — the only model with a surviving, building, test-passing S13 artefact. deepseek's was
  lost to the loop's retry-reset; codex produced nothing.

## Eval table

Loop never reached `verified` for ANY slice/model — turn-cap + escalation-cascade ensured 100% loop
FAIL. So "completed?" = did the loop reach `verified`; "supervisor pass/fail" = my independent judgement
of the preserved code against the spec ACs. Durations approximate; token/cost telemetry unreliable
(cost=0 known issue; not reported).

| Slice | Model | Loop completed? | Rework (resolve/escalate) | Supervisor pass/fail | Notes |
|---|---|---|---|---|---|
| S08 | codex | no | n/a | FAIL (no output) | gpt-5.3-codex HTTP 520 |
| S08 | deepseek | no (turn-cap) | multiple | **PASS (prod)**, test missing | 11 drivers+registry+run.go fail-fast; suites green |
| S08 | glm | no (turn-cap) | multiple | FAIL | 7/11 drivers, no fail-fast, no registry |
| S01 | codex | no | n/a | FAIL (no output) | HTTP 520 |
| S01 | deepseek | no (turn-cap) | multiple | FAIL | leaf only, not wired to worker.go, no test |
| S01 | glm | no | multiple | FAIL (no output) | slowest; nothing produced |
| S13 | codex | no | n/a | FAIL (no output) | HTTP 520 |
| S13 | deepseek | no (turn-cap) | multiple | FAIL (lost) | good early attempt wiped by retry-reset |
| S13 | glm | no (turn-cap) | multiple | **partial PASS** | validator+test build & pass; not wired to Write() |

## Three-way verdict

**Per slice:** S08 → deepseek (clear). S01 → deepseek (only output). S13 → glm (only surviving artefact,
and it wrote a passing test). **Overall implementer ranking: deepseek-v4-pro > glm-5.2 > gpt-5.3-codex.**
- **deepseek-v4-pro**: the model I'd run Sworn with today. It produced the most complete, correct,
  regression-free code, converged often enough to reach `verifying` once, and was the fastest of the
  three. Weakness shared with all: builds leaves without wiring integration points (Rule-1), and is
  verbose enough to brush the 25-turn cap.
- **glm-5.2**: viable via ollama-cloud and the only one to ship a passing test, but slower and less
  complete per attempt; missed core integration/outcome ACs.
- **gpt-5.3-codex**: unusable in this build — the model id returns HTTP 520 on the openai-responses
  endpoint, and that driver has a separate Responses-API multi-turn bug. Zero output. (This is a
  build/driver/model-availability problem, not necessarily a capability statement about the model.)

**Is the model the bottleneck? No — Sworn is.** Every slice failed in the loop for engine reasons
(cold-start gaps, 2 SIGSEGVs, the universal content-omitempty serialization bug, the 25-turn cap, the
retry-reset, the openai-only escalation cascade, phase fail-fast), not model-quality reasons. With the
two trivial fixes applied, the best model still couldn't get a slice to `verified` because of the
turn-cap and cascade.

**Is Sworn sound enough to build more unattended? NO — not currently.** `sworn run --parallel` could not
cold-start a planned release at all (it depends on Driver-1 `/implement-slice` bootstrap it never
performs), then crashed twice with nil-factory SIGSEGVs proving the parallel path had never run from the
real CLI, then hit a universal agent-loop serialization bug. After supervisor fixes it runs but cannot
converge: implementers brush the 25-turn cap, retries discard good work, and the first retry-exhaustion
cascades (via openai-only escalation + phase fail-fast) into a total release FAIL. The autonomous loop is
NOT yet trustworthy for unattended delivery. The deterministic core the audit praised (router, worktree
materialisation, state machine, phase scheduling) does work — the failure is concentrated in the
agentic dispatch/verify/resilience layer, exactly the FT-1/FT-2/FT-3 tracks this release exists to fix.

**Loop verdicts vs reality (FT-3 confirmed):** the loop reported FAIL for everything, but deepseek's S08
was actually production-correct and glm's S13 actually passed tests. The loop's terminal states reflect
its own brokenness, not implementation quality — re-running tests independently was essential.
