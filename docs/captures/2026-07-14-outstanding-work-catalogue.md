# Outstanding work catalogue — 2026-07-14

**Purpose.** A single, live-state inventory of everything already planned, in flight, or
catalogued but not delivered. The architectural review (see
`2026-07-14-architecture-review-brief.md`) must **read this first and cross-reference every
finding against it**, so it reports *new* problems rather than re-discovering known ones.

**This is not hypothetical.** On 2026-07-14 the engine's `--upstream --tag` SHA-guard bug was
diagnosed and fixed from scratch — and only afterwards found to be **sworn#26, already filed**.
A backlog nobody reads is not a backlog; it is a second, slower way of finding the same bug.
Preventing that recurrence is a first-class goal of the review.

---

## 1. Merged (2026-07-14) — do NOT report these as findings

Both landed on `main` @ `637d05c`.

| PR | What it landed |
|---|---|
| **sworn#105** | Baton **v0.12.0**: the six LLM-check prompts vendored (143 lines of inline Go constants deleted); `llm-check-report-v1` **GRADED**; project context detected, not hardcoded. **Fixed sworn#26** (`--upstream --tag` SHA guard blocked every legitimate version bump) and a `WriteUpstreamPin` bug that left the pin claiming one version while carrying another's SHA. |
| **sworn#107** | Baton **v0.13.0**: declared, human-ratified project context + stakes (`.sworn/project.json`, fail-closed to HIGH). **Unified role-model resolution** (`config.ResolveRoleModel` — one function, four roles; planner + captain added; **no env layer, no hardcoded models**; both copies of `DefaultEscalationModels` deleted). **XDG credentials** (`internal/model/credentials.go` — `credentials.json` 0600, canonical env names, `SWORN_` prefix dropped, migration + `sworn doctor` reporting). Live LLM-check tests + a nightly `live.yml` workflow. |

**Closed by these:** sworn#26. **Read the code, not your priors** — the pre-merge shape of
`internal/config`, `internal/model`, `internal/project` and `internal/gate/llmcheck.go` no longer
exists.

## 1b. Commissioned, not yet built

| Brief | Scope |
|---|---|
| `2026-07-14-local-and-cloud-providers-brief.md` | Ollama Cloud, local Ollama (loop-capable), llama.cpp, LM Studio, vLLM, LocalAI. A **declared endpoint table** replacing `provider.go`'s 16-case switch, plus a **live endpoint-conformance suite** that derives each provider's OAI *dialect quirks* from observation. **Closes #15.** |
| `2026-07-14-architecture-review-brief.md` | This review. |

### Architecture review outputs (2026-07-14)

The guard-first review is tracked by **#108**. Its remediation epic is **#109**
and the drafted release board is
`docs/release/2026-07-14-autonomous-operations/` (12 planned slices). That board
orders terminal truth and cancellation before the command/event core, durable
paging, the responsive mobile board, authenticated controls, and the final
assembled real-binary journey.

**Protocol side:** Baton **v0.12.0** and **v0.13.0** are merged and tagged.

## 2. Specced, not implemented — the `2026-07-11-contract-edge-gates` release

Twelve slices across five tracks, **all specs written and committed** on
`release-wt/2026-07-11-contract-edge-gates`. State: `planned`. Next step is
`/implement-slice` per track.

| Track | Slices |
|---|---|
| **T1-lint-contracts** | `S01-lint-contracts-registry`, `S02-lint-contracts-mock-parity` |
| **T2-assemble** | `S03-assemble-command` — `sworn assemble` is currently **unimplemented**; the Rule 10 assembly gate is advisory-absent because of it |
| **T3-capability-selection** | `S04-provider-registry`, `S05-capability-eligibility`, `S06-routing-preferences` (ADR-0013; serial) |
| **T4-loop-fidelity** | `S07-resume-worktree-reset`, `S08-dry-run-parallel-honored`, `S09-loop-max-turns`, `S10-autonomous-design-authority` |
| **T5-verify-fidelity** | `S11-guard-fidelity-gate` (Rule 12), `S12-mock-code-construct` (Rule 10) |

**The review must not propose work that duplicates these twelve slices.** Several are
architectural by nature (the provider registry, the capability-eligibility gate, the
guard-fidelity gate).

---

## 3. Known-architectural issues already filed

These are the ones a review would most plausibly "discover". They are **already catalogued**.
Cross-reference before reporting.

| Issue | Class |
|---|---|
| **#15** self-registering provider factory to eliminate `provider.go` touchpoint collision | duplication / extension point |
| **#22** replace bespoke string scanners over structured docs with marshaller round-trips + write-time validation | contract drift / prose-scraping |
| **#89** unify `google.go` / `bedrock.go` duplicate pricing lookups into the S08 registry | duplication |
| **#79** historical ADR-0011 phantom-contract issue; the file was backfilled in `957de6d`, but the issue remains open/stale | record hygiene |
| **#70** agent-loop cost telemetry is nominal flat $2/1M; the pricing registry is **dark code** | dead code |
| **#68** `PauseEngine` is **dark code** — no CLI/MCP/TUI surface can trigger it | dead code |
| **#67** dead `example.com` `$schema` literals in code + stale artefacts on disk | dead code |
| **#66** `WriteBoard` validates **after** writing `board.json` to disk | fail-open ordering |
| **#82** TUI board load runs four gates **synchronously in `Update`** — UI locks up (21.5s measured) | layering / blocking I/O |
| **#81** TUI board reads the primary working tree, not the git-ref oracle | single-source drift |
| **#75** `doctor` pin-currency check is a layout heuristic: `[OK]` on a pin one release behind | silent-green guard |
| **#76** ship gate passes **vacuously** (exit 0) when the impact heuristic matches zero journeys | silent-green guard |
| **#62** `generateProof` emits boilerplate reachability + an overclaiming Delivered list | overclaiming |
| **#61** triage conflates `INCONCLUSIVE` with `FAIL` — re-dispatches the paid implementer on infra noise | error taxonomy |
| **#60** verification record fidelity: FAIL drops typed violations; fresh-context flag hardcoded `false` | record fidelity |
| **#63/#64** Rule 11 residuals: gate helpers run `git diff` in ambient cwd; `finishTrack` swallows `git push` failure | process-global state |
| **#93** parallel loop: a verified slice never commits its verdict → router re-dispatches verify forever | loop correctness |
| **#31** rename OpenAI provider prefixes (`openai/` → `/v1/responses`) | naming/contract |
| **#29** ADR: namespace release artefacts under `docs/sworn/` to avoid consumer-repo collision | layout |

### Newly filed by architecture review #108

| Issue | Class / planned owner |
|---|---|
| **#109** autonomous operations plane: durable loop control, mobile board, notifications | remediation epic / drafted release; authority boundary ratified |
| **#110** benchmark accepts invalid ground truth as PASS | guard-domain integrity |
| **#111** notification delivery can hang loops; no durable outbox | S07–S08 |
| **#112** specquality gives current-format specs 100% with zero examples | guard-domain integrity |
| **#113** runtime control surfaces bypass loop state ownership | S04–S06 |
| **#114** parallel loop silently substitutes legacy routing after oracle failure | S02 |
| **#115** terminal labels can outrun durable effects | S01 |
| **#116** subscription CLI drivers inherit unrelated parent secrets | subprocess security |
| **#117** dependency policy/direct-module ADR ownership is contradictory | policy ratified; implementation/governance guard open |
| **#118** telemetry init consent is unreachable; opt-in contract drift | explicit opt-in/value-led invitation ratified; implementation open |

**81 open issues in total.** The full list is authoritative:
`gh issue list --repo swornagent/sworn --state open --limit 100`.

---

## 4. Rule 2 deferrals logged today (2026-07-13/14) — tracked, not yet done

From the proof bundles in `docs/captures/`:

- ~~The stakes gate is not proved end-to-end against a live model.~~ **CLOSED** (sworn#107).
  `internal/gate/llmcheck_live_test.go` proves it against `openai/gpt-4.1-mini`: a real model
  emits schema-valid `llm-check-report-v1` with `blocking`, and the *same diff* is graded
  `low`/advisory at low stakes and `medium`/**BLOCKING** at high stakes. The stakes keying does
  real work.
- **`sworn init`'s interactive elicitation shell has no automated test.** The pieces beneath
  it (`Elicit` parse, `Save` validation, `Resolve` fail-closed) are all covered.
- **`LLMCheckReport` is not emitted as an `llm-check-report-v1` record to disk.** The schema
  grades the model's *response*; the engine's struct keeps sworn-native fields.
- **The TUI degrades rather than fails closed** on an unresolvable board record (deliberate:
  a read-only view; the gates fail closed).
- **Severity-vocabulary reconciliation is complete in Baton v0.12.0** — no longer outstanding.

---

## 5. Known-stale / contradictory records (a review will trip over these)

- **#79 is stale.** ADR-0011 was originally missing but was backfilled in commit
  `957de6d`; the open issue should be reconciled rather than treated as a current
  missing-file defect.
- **#73 — the vendored advisory AGENTS fragment is stuck in the seven-rule era** (pre-Rules
  8–12) in some surfaces.
- **#74 — `internal/prompt/VERSION.txt` is dead provenance** contradicting the actual vendor
  state.
- **`docs/release/` contains ~20 historical releases.** Their `design.md` / `spec.md` records
  describe *intended* architecture. Today proved these drift from the code (S08 said "keys in
  the XDG config file, 0600"; the drivers said canonical env names; neither shipped that way).
  **They are evidence, not truth** — treat a design record as a claim to verify against code,
  not as a description of it.

---

## 6. Public-safety constraint (hard)

The repo is **public**. `scripts/public-safe-scan.sh` runs in CI and fails the build on
identity leaks. It catches `/home/<user>`, `/Users/<user>`, the product name, private repo
names — but it **cannot** catch the consumer repo's bare directory name, because that name
collides with a common English verb.

**26 files still reference the downstream consumer repo by its local directory name and are
already public.** Not caught, not deliberately kept — the guard simply cannot see them. A
review touching `docs/` must not add more, and closing the existing 26 is a candidate task
(manual sweep + a rename; a regex cannot do it).
