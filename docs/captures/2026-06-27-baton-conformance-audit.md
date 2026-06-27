# Baton Conformance Audit — SwornAgent

**Date:** 2026-06-27
**Scope:** Does SwornAgent implement the Baton rule-set (Rules 1–11), records-as-JSON, orchestration engine, role ontology, model/provider layer, and contract/vendor conformance in their entirety?
**Method:** Per-dimension auditor findings, each re-checked by a fresh-context adversarial verifier against live repo state. This report uses the **corrected** verdicts/severities, not the originals.

---

## 1. Verdict

**No. Sworn does not implement Baton in its entirety.** It has built a genuine, mostly-faithful *deterministic* core — a git-ref oracle reader, a pure-function slice router, a topological scheduler, a goroutine fan-out, a fail-closed state machine, and competent Rule 8/9/10 *leaf* gates — and it embeds all four canonical role prompts plus all eleven advisory rule texts at high fidelity. But the conformance failures are structural and concentrated in the load-bearing CRITICAL rules.

The most material gaps, headline-first:

1. **Records-as-JSON is unbuilt as a layer.** There is *no JSON-schema validation anywhere* in the Go tree, *no* canonical schema vendored or embedded, and the records' `$schema` is a hardcoded `example.com` placeholder. `proof.json` and `spec.json` do not exist as records (still markdown); `board.json` does not exist (the oracle still parses `index.md` YAML frontmatter — the exact corruption surface ADR-0009 exists to kill); `journeys.json`/`attestations.json` are emitted but unvalidated and shape-divergent from canonical.
2. **The Rule-6 proof bundle is hand-templated, not emitted-and-validated.** `not_delivered` and `divergence` are hardcoded `"None"`; reachability/delivered are constant self-referential boilerplate; the proof-bundle verification gate is *optional* and never fails closed.
3. **The autonomous loop's verifier is not Rule-7-grade.** `sworn run` collapses implement→verify→commit-verified into one process with a single-shot tool-less SPEC+DIFF judge; the fresh-context, test-re-running agentic verifier exists only as inert MCP/slash-command text the engine never dispatches.
4. **The agentic interpreter is absent.** Sworn routes only on typed `status.json` state; any outcome not captured as a clean state transition stalls/pauses for a human. (Correctly placed in Sworn-not-Baton, but unformalized.)
5. **Rule-10 gates run only as opt-in CLI subcommands** — none is wired into the merge/release loop; the no-mock detector is blind to the entitlement boundary the seed journey crosses.

Vendor staleness compounds all of the above: the pin (`9ae08fb`) predates records-as-JSON *and* the tool-neutral `baton/` layout, three inconsistent VERSION strings coexist, and `doctor` is structurally blind to pin staleness.

---

## 2. Conformance map

### Rules 1–5 (advisory text + wiring)
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| Rule 1 — Reachability (text + surfacing) | thin | high | `sworn://baton/rules` advertised in init AGENTS.md but not registered as an MCP resource — dead link |
| Rule 2 — No Silent Deferrals | thin | medium | No dark-code/TODO detector; `open_deferrals` is free-text, wired only to the Rule-10 mock gate |
| Rule 3 — Capture Discipline | thin | medium | Text full; `Materialise` dead/deprecated; canonical delivery path (MCP rules) is a dead link |
| Rule 4 — Commit Messages | thin | low | Text present; zero surfacing — auto-commits write single-line bodies, never inject the rule |
| Rule 5 — Session Discipline | **full** | low | Text at fidelity across 3 copies + adopt/doctor wiring; docs/release divergence is in-band exception |

### Rule 6 — Proof Bundle
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| Emit proof.json (proof-v1) | missing | high | No proof.json emitted anywhere; proof.md hand-authored (spec inverts this) |
| Validate proof.json vs schema | missing | high | No proof-v1 schema embedded; no validator; nothing checks completeness |
| Render proof.md from record | thin | medium | Hand-templated; constant sections + dangling `scripts/release-verify.sh` false claim |
| Fail-closed verification gate | missing | critical | Proof is "optional"; missing/empty/malformed proof never fails the verdict |
| Run loop requires proof before `implemented` | thin | high | File-existence side effect; thin/empty impl reaches `implemented` with green proof |
| Section: files_changed | thin | low | Live git, but `git status --porcelain` (working tree) not `git diff --name-only <base>`; free-text not array |
| Section: test_results | thin | medium | Live but unstructured + full-suite (`go test ./...`) vs slice-scoped per-command |
| Section: reachability | thin | high | Constant boilerplate about Sworn's own test — the Rule-1 anti-pattern baked in |
| Section: delivered | thin | high | Constant 3-item meta-list; never derived from acceptance criteria |
| Section: not_delivered | missing | high | Hardcoded `None`; structurally cannot enumerate deferrals |
| Section: divergence | thin | medium | Hardcoded `None`; cannot reflect plan drift |
| Vendor staleness of proof prompts | thin | high | Embedded implementer.md/verify-stateless.md encode proof.md-primary + PROOF-optional |

### Rule 7 — Adversarial Verification
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| In-loop verifier (engine path) | thin | high | Single-shot tool-less judge; no test re-run, no proof-v1, no live repo; PASS drives →verified |
| Agentic verifier.md wired into loop | thin | high | Conformant verifier exists only as MCP/slash text; engine never dispatches it; v0.4.2 stale |
| Fresh-context boundary enforced | thin | high | Producer+consumer share one process; standalone `sworn verify` doesn't mutate status.json |

### Rule 8 — Requirements Fidelity
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| Records-as-JSON: gates read spec.md not spec.json | thin | high | Zero code reads spec.json/acceptance_criteria; regex-scrapes markdown |
| Trace gate fails closed | thin | high | Advertised AC→test link absent from lint path; vacuous (needs:0) on real release |
| Two divergent RTM impls | thin | high | `lint trace` vs DoR disagree; 20 false-positive EARS FAILs on Sworn's own release |
| RTM chain inert on real intake | thin | critical | needs:0 — parser matches only synthetic `N-NN:`; dropped-need detection never fires |
| covers_needs minItems:1 | thin | medium | Empty covers_needs passes both paths; rtm.Build ignores covers_needs entirely |
| EARS notation enforcement | thin | high | Requires literal "THE SYSTEM SHALL"; rejects 100% of spec's own reference examples |
| Definition-of-Ready gate | thin | high | Wired to design_review edge only, not the spec-mandated planned→in_progress edge |
| Coverage gate | **full** | low | Runs, maps every AC, fails closed; heuristic token-overlap is a robustness limit only |
| Spec-quality first-pass | thin | medium | Mutation completeness works; "missing intake detail" structural check unimplemented |
| Top-level `sworn trace`/`coverage` | thin | low | Spec names `sworn trace`; only `sworn lint trace` exists (naming/discoverability) |

### Rule 9 — Design Fidelity
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| Type-1/2 state types | **full** | low | Field-for-field match to canonical record |
| designfit — Type-1-without-decision | **full** | low | Deterministic fail-closed, wired, tested |
| designfit — arch-significant-but-Type-2 | **full** | low | Deterministic fail-closed, wired, tested |
| designfit — empty-decisions fallback | thin | medium | Hardcoded Sworn-only path prefixes; inert for any other project |
| designaudit — Layer-1 checks | **full** | low | Three categories present, run, tested |
| designaudit — Layer-2 cohesion | **full** | low | Human verdict correctly fail-closed before exit 0 |
| designaudit — token-config escape hatch | thin | medium | `lint design` honours inline tokens; reads inline array not token_source file; `designaudit` cmd token-blind |
| designaudit — per-line allowlist | **full** | low | design-allowlist.json IS auto-read in internal/gate (auditor checked wrong package) |
| designaudit — scans diff vs whole tree | thin | high | Walks whole project; swamps first-slice verification with pre-existing literals |
| designaudit — test-file exclusion | missing | medium | No `*.test.*`/`__tests__/` exclusion — false positives where prompt promises none |
| designaudit — CLI flag mismatch | thin | high | Vendored verifier.md invokes `--slice/--release/--worktree`; binary rejects them |
| designaudit — Rule-2 deferral path | thin | medium | Verifier-owned (by spec); broken flag invocation leaves it un-exercised |
| Captain design-review Step 2b | **full** | low | Byte-identical port, embedded, served, gate is real |
| Design-system declaration + ui_bearing fail-closed | **full** | low | Matches three-tier concept + fail-closed-on-missing |

### Rule 10 — Customer Journey
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| journeys-v1 durable record | thin | high | Flat ratification vs nested; +regression_test_path; would FAIL canonical validation; unchecked |
| Elicitation (model drafts) | thin | medium | Directory-scan template, not model-driven; Rule-2 deferral lacks tracking/ack |
| Journey gate fail-closed before merge | thin | critical | Exists as standalone command; NOT wired into merge loop; no journeys.json in repo |
| No-mock-boundary enforcement | thin | critical | Zero entitlement keywords; never invoked by loop/verify; constitutive boundary detached |
| attestations-v1 + cutover gate | thin | high | Flat shape diverges; zero-touched/missing-file bypass passes green; ship not wired to merge |
| Impact analysis | **full** | medium | Fail-closed, derived from touchpoints, over-inclusion bias; behavioural-surface blind spot acknowledged |
| SEED keyless-subscription journey | missing | high | No journeys.json; template can't draft it; entitlement boundary unprotected |

### Rule 11 — Process-Global Mutation
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| Scoped mutation (git core) | **full** | low | cmd.Dir everywhere, never os.Chdir; single chokepoint |
| Fail-closed target assertion | thin | medium | Guards only `Dir != ""`; never os.Stat exists / expected-repo |
| Reachability artefact (empty-Dir guard) | **full** | low | Real test proves guard fires + ambient repo unmutated |
| Target assertion before worktree-add (parallel) | thin | medium | absRoot never asserted before `git worktree add` |
| Target assertion + merge error handling (scheduler) | thin | high | PrimaryWorktreeRoot unasserted; follow-on merge error silently swallowed |
| Ambient env mutation (designaudit/env) | thin | medium | Ambient os.Setenv, best-effort/no restore, not goroutine-safe, no Rule-2 note |
| Sworn enforces Rule 11 for slices it builds | missing | high | No gate/lint detector for unrestored chdir/setenv or unasserted git-with-dir |
| Verifier/Captain prompts enforce guard-demo | missing | high | Neither prompt operationalises Rule 11 (partly inherited from canonical) |
| Test-suite mutations restored | thin | low | t.Chdir/t.Setenv partial; legacy raw os.Chdir+defer remains (no t.Parallel, so latent) |

### Records-as-JSON
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| slice-status-v1 (status.json) | thin | medium | Emitted+used but unvalidated; example.com $schema placeholder |
| board-v1 (board.json) | missing | critical | No board.json; oracle parses index.md frontmatter (the corruption surface) |
| spec-v1 (spec.json) | missing | high | No spec.json writer; Rule-8 contract is dual-authored markdown |
| proof-v1 (proof.json) | missing | high | No proof.json record; Rule-6 bundle stays free-text |
| journeys-v1 (journeys.json) | thin | medium | Emitted JSON, no $schema, divergent shape, unvalidated |
| attestations-v1 (attestations.json) | thin | medium | Read/consume only; no $schema; flat-vs-nested boundary divergence |

### Orchestration Engine
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| Oracle reader (git-ref) | **full** | low | Keystone present, tested, wired into live router |
| Slice router (deterministic) | **full** | low | Faithful pure-function port of captain-route tree |
| Scheduler (topological phases) | thin | medium | Phase ordering yes; dependent track branches release-wt that may lack T1's merged code |
| LLM interpreter (dispatch_and_interpret) | missing | high | No analogue; any non-typed outcome stalls/pauses to human |
| Slice state machine | thin | medium | Graph faithful but implemented→verified in one process; verifier_was_fresh_context never set |
| In-loop verifier fresh-context | **full** | low | Stateless judge payload excludes implementer transcript = context separation (spec-conformant); Verification.Model records wrong id |
| Track-mode parallelism | thin | high | Invariant-1 enforced; invariant-2/4 (touchpoint disjointness, conflict BLOCK) not enforced in loop |
| Pause / resume | thin | medium | Coarse halt-then-rerun; findFirstNonTerminal blind, works only by router correction |
| Crash recovery (Gap B) | thin | medium | PID-reap + committed-state resume; no error_max_turns→PAGE; no cross-run breaker |
| Merge gates | thin | high | Verified-check reads working-tree status.json not oracle; no invariant-4 classifier; MCP-only |
| Orchestrator decision-log | missing | medium | Routing/triage reasoning is stderr-ephemeral; only durable audit is process-ownership events |

### Role Ontology
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| Implementer role | **full** | low | Real Baton prompt drives engine; honours implemented checkpoint |
| Verifier role | thin | high | Engine runs Sworn-authored stateless judge; canonical agentic verifier only via slash/MCP |
| Planner role | thin | medium | Present-but-passive; no engine dispatch (prompt-resource only) |
| Captain design-review function | **full** | low | Built + engine-wired (refutes seed "never called"); minor parsePins under-parse |
| Captain release-orchestrator conflation | thin | medium | captain.md claims orchestrator identity realised in code, not the prompt |
| Orchestrator role (unformalized) | missing | medium | Coordinator exists as deterministic Go but is unnamed, unformalized, no recorded design choice |
| Vendored captain.md staleness | thin | medium | spec.md refs (v0.4.2) vs canonical spec.json; no MCP-served captain so blast radius narrow |

### Model / Provider Layer
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| Agentic Chat coverage | thin | high | Only OAI + OpenAIResponses implement Chat; capability checked at runtime not resolution |
| Capability matrix | thin | medium | Capability implicit in method-satisfaction; no introspectable descriptor |
| Keyless CLI coverage (Gap A) | thin | medium | claude-cli verify-only; codex stub; cost always 0 |
| Error{Kind} taxonomy — production | **full** | low | Typed kinds, HTTP classification, 8 drivers route through it |
| Error{Kind} taxonomy — consumption | thin | high | Loop never branches on terminal vs transient; KindAuth retried+escalated |
| Self-registering factory (sworn#15) | thin | low | Hardcoded switch; ErrDriverNotRegistered name over-states |
| Verifier role fidelity (judge vs agentic) | thin | medium | Stateless judge vs canonical repo-walking test-re-running verifier |

### Contract Conformance
| Element | Verdict | Severity | One-line gap |
|---|---|---|---|
| Schema validation on write | missing | critical | No JSON-schema validation anywhere; placeholder $schema; nothing fails closed |
| Vendor pin honesty/currency | thin | high | Pin 9ae08fb predates records-as-JSON + baton/ layout; embed ships pre-JSON prompts |
| VERSION-string consistency | thin | medium | Three strings (v0.4.2/v0.5.0/v1.0.0); but orphaned files not surfaced — latent rot |
| doctor embed-integrity | **full** | low | Presence/length/headings checked + fail-closed |
| doctor pin-currency drift | missing | high | Only asserts semver tag; never compares SHA vs HEAD or detects pre-JSON prompts |
| Vendor source map currency | thin | medium | Map current on baton/ layout but pin SHA has no baton/ dir — re-vendor would fail closed |

---

## 3. Prioritized gap list (critical-first)

Each entry: gap → recommendation → file:line. **⚠ OVERTURNED** marks findings where the adversarial verifier changed the auditor's verdict/severity (the over- or under-stated ones — read these first).

### CRITICAL

1. **Fail-closed proof-bundle gate is missing** — `sworn verify` PASSes a slice with no proof bundle; proof read with swallowed error, optional in payload. → Make proof mandatory: fail closed (Blocked/Fail) when missing/unreadable/empty/invalid before model dispatch; drop "optional" from prompt + CLI help. `internal/verify/verify.go:34,51-54,106-116`; `internal/prompt/verify-stateless.md:75`.

2. **Schema validation on record write is absent** — no JSON-schema validation anywhere; `$schema` is example.com placeholder; malformed/drifted records written silently. → Vendor `baton/schemas/*-v1.json`, add fail-closed validation in `state.Write` and the proof/spec/journey writers. `internal/state/state.go:184-192`; `internal/run/run.go:293`.

3. **board-v1 unbuilt; oracle parses index.md frontmatter** — the exact newline-fusion corruption surface ADR-0009 exists to eliminate. → Build board.json as oracle source of truth, render index.md from it with the drift guard. `internal/board/index.go`, `oracle.go:370-394`.

4. **RTM horizontal chain inert on real intake (needs:0)** — needs-parser matches only synthetic `N-NN:`/single-line-bold; real planner output yields zero needs, so dropped-need detection never fires. → Move needs to a stable machine-readable form; add integration test asserting needs>0 on a real release. `internal/gate/trace.go:325-385`; `internal/rtm/rtm.go:345-358`.

5. **Journey gate not wired into merge loop** — fail-closed gate exists only as a standalone command; a release can merge with no journeys artefact. → Invoke `journey.Check` + ship gate from merge-release/merge-track. `internal/run/parallel.go`, `internal/router/router.go:381-429`, `cmd/sworn/journeys.go:73-104`.

6. **No-mock detector blind to entitlement + detached from loop** — zero entitlement/subscription keywords; never invoked by the autonomous loop or verify. → Add entitlement patterns; wire `RunMock` into per-slice verify. `internal/gate/mock.go:118-178`; `cmd/sworn/lint.go:461-499`.

### HIGH (selected — see map for the full set)

7. ⚠ **OVERTURNED (missing → thin, critical → high): In-loop verifier not Rule-7-grade** — single-shot tool-less judge; no test re-run, no proof-v1, no live repo; PASS drives →verified. Verifier downgraded because a fail-closed LLM judge *does* exist and is correctly ordered behind the implemented checkpoint, but lacks mechanical teeth. → Demote to labelled first-pass; require a separate fresh-context dispatch before `verified`, or run proof.json test commands deterministically. `internal/verify/verify.go:85`; `internal/run/slice.go:412-429`.

8. **Agentic verifier.md never dispatched by engine** + vendored verifier.md is v0.4.2 stale (proof.md/spec.md). → Re-vendor from canonical; give engine an agentic verifier path or document `sworn run` PASS as provisional. `internal/agent/agent.go:6-7`; `internal/prompt/VERSION.txt`.

9. ⚠ **OVERTURNED (missing → thin, critical → high): Fresh-context boundary** — producer+consumer share one process, but the stateless judge payload excludes the implementer transcript, which *is* the context separation Rule 7 names. Residual: no enforced session separation; `sworn verify` doesn't mutate status.json. `internal/run/slice.go:411-430`; `cmd/sworn/verify.go:64-75`.

10. **proof.json / spec.json records missing** (Rule 6/8 contracts dual-authored markdown). → Define spec-v1/proof-v1 record types, emit-validate-render. `internal/implement/implement.go:40`; `internal/mcp/tools_plan.go:48-50`.

11. **proof.md sections constant/self-referential** — reachability boilerplate (Rule-1 anti-pattern), delivered meta-list, not_delivered hardcoded `None`. → Derive every section from live state + acceptance criteria; remove the dangling `scripts/release-verify.sh` line. `internal/implement/implement.go:177-191`.

12. **EARS rejects the spec's own reference examples** — requires literal "THE SYSTEM SHALL". → Adopt one matcher on the canonical litmus (keyword + `shall`, system slot optional). `internal/ears/ears.go:109`; `internal/gate/trace.go:97`.

13. **DoR gate wired to wrong edge** — gates design_review→in_progress, not the spec-mandated planned→in_progress; also bypassed when design.md generation fails. → Invoke `CheckDoR` on the planned edge. `internal/implement/implement.go:52-78`.

14. **Two divergent RTM parsers** — `lint trace` emits 20 false-positive EARS FAILs on Sworn's own release (continuation-line join divergence). → Unify on the multi-line-aware classifier. `internal/gate/trace.go:417-437` vs `internal/ears/ears.go:263-310`.

15. **designaudit scans whole tree + broken CLI flags** — walks whole project (swamps first-slice verify); vendored verifier.md passes `--slice/--release/--worktree` the binary rejects. → Diff-scope the scan; re-sync vendored verifier.md or add the flags. `internal/designaudit/designaudit.go:173`; `internal/prompt/verifier.md:169`.

16. **LLM interpreter absent** — non-typed outcomes stall/pause. → Either formally defer (Rule 2) as the hosted-layer value-add, or add a bounded interpreter step. `internal/scheduler/worker.go:249-261`.

17. **Merge gate reads working-tree status.json not the oracle** — re-opens the false-ready bug class. → Route the verified-check through `board.Oracle`; add invariant-4 conflict classifier. `internal/mcp/tools_ops.go:410-446`.

18. **Error{Kind} taxonomy unconsumed by loop** — KindAuth/KindCredits retried + escalated like transient. → Add ErrorKind/IsTerminal to `orchestrator.Input`; Halt on terminal. `internal/orchestrator/triage.go:39-57`; `internal/run/slice.go:321-327`.

19. **Agentic Chat only on 2 driver types; checked at runtime** — `--implementer-model anthropic/...` fails mid-run. → Capability check at `ResolveImplementerModel`. `internal/run/run.go:343-352`; `internal/config/config.go:225`.

20. **journeys-v1 / attestations-v1 shapes would fail canonical validation; never validated.** → Align to nested ratification{}/boundary{}, add $schema + validate-on-write. `internal/journey/journey.go:76-98`, `walkthrough.go:31-55`.

21. **Sworn doesn't enforce Rule 11 for slices it builds; prompts omit guard-demo; doctor blind to pin staleness.** → Add a chdir/setenv/git-with-dir gate; add a doctor SHA-vs-HEAD + pre-JSON-prompt drift check. `internal/gate/*`; `cmd/sworn/doctor.go:419-449`.

22. **Vendor pin predates records-as-JSON + baton/ layout.** → Re-vendor from canonical HEAD; bump pin + VERSION; land centralisation. `internal/adopt/baton/VERSION`.

### Notable OVERTURNED (verdict softened by the verifier — auditor *over-stated*)

- ⚠ **Rule 5 Session Discipline: thin → FULL.** Auditor tested induction.go (wrong surface); text is at fidelity with working adopt/doctor wiring and an in-band docs/release exception.
- ⚠ **files_changed: full → thin.** Auditor scored full; verifier found `git status --porcelain` (working-tree, empties after commit) vs spec's `git diff --name-only <base>`, plus free-text not array.
- ⚠ **designaudit per-line allowlist: thin/high → FULL/low.** Auditor grepped only `internal/designaudit/`; the allowlist IS auto-read in `internal/gate/archrules.go`.
- ⚠ **designaudit token-config: missing → thin.** Auditor inspected `sworn designaudit`; `sworn lint design` DOES implement the escape hatch (inline tokens, not the token_source file).
- ⚠ **not_delivered: critical → high; divergence: missing/high → thin/medium.** Constants confirmed, but the Rule-7 verifier reads git diff + spec directly, so the failure is over-strict (false-FAIL) not silent-pass.
- ⚠ **In-loop verifier fresh-context (engine): full.** Auditor called it broken self-certification; verifier credited the transcript-excluding payload as genuine context separation.
- ⚠ **Slice state machine: full → thin.** Auditor scored full; verifier flagged the implemented→verified one-process collapse + dead `verifier_was_fresh_context`.
- ⚠ **Scheduler dependent-track: full → thin.** Phase ordering present, but dependent track branches a release-wt that may lack T1's merged code.
- ⚠ **status.json: high → medium.** The marshaller can't emit malformed structure; missing validation is a robustness post-condition for the one record that round-trips.

---

## 4. Foundation-track scope (post-R3 combined release)

Grouping the confirmed thin/missing gaps into tracks for the next release:

**Track FT-1 — Orchestration / Engine.**
- LLM interpreter (`dispatch_and_interpret` analogue) — bounded cheap-model step between dispatch and router poll, or a formal Rule-2 deferral.
- Orchestrator decision-log / captain-trial-log — persist router `Decision` + triage `Output` to the supervisor SQLite (extend events → decisions).
- Crash recovery: error_max_turns→PAGE + cross-run failure-fingerprint circuit breaker.
- Scheduler dependent-track: branch from dependency tip or auto-merge to release-wt at finishTrack.
- Merge gate: route verified-check through oracle; add invariant-4 conflict classifier; `sworn merge-track/merge-release` CLI.
- Track-mode invariant-2 (touchpoint disjointness) enforcement in the loop.
- Pause/resume: make findFirstNonTerminal read committed state.

**Track FT-2 — Model-layer service layer.**
- Capability descriptor (`Capabilities()`/registry) + fail-fast at implementer-model resolution.
- Error{Kind} consumption in triage (terminal → Halt).
- Self-registering factory (sworn#15) or rename the sentinel.
- Agentic Chat for native Anthropic (+ keyless Chat over `claude -p`, or formal verifier-only deferral; ship/defer codex sworn#19; fix cost=0).

**Track FT-3 — Agentic loop verifier (Rule 7).**
- Engine path that dispatches the agentic `verifier.md` (test re-run + live repo + worktree anchoring) OR demote in-loop judge to labelled first-pass and require a separate fresh-context certify.
- Set `verifier_was_fresh_context` honestly; fix `Verification.Model` to record the verifier model.

**Track FT-4 — Records-as-JSON.**
- Embed `baton/schemas/*-v1.json`; add fail-closed validate-on-write to all writers; replace example.com $schema.
- Build board.json (+ render index.md, drift guard), spec.json, proof.json record types.
- Align journeys/attestations to canonical nested shapes + $schema + validation.

**Track FT-5 — Role ontology.**
- Formalize the deterministic **Orchestrator** role (Sworn-side spec; record the deterministic-vs-agentic choice as a Type-1 design decision).
- Split the **Captain** artefact: Rule-9 design-reviewer (built) vs release-orchestrator (realised by the deterministic engine, not the prompt).
- Re-vendor planner/implementer/verifier/captain from canonical post-records-as-JSON; bump VERSION.txt.

**Track FT-6 — Contract / re-vendor.**
- Bump pin to a commit containing the `baton/` layout (≥ records-as-JSON HEAD) so source map + pin are coherent.
- Centralise the three VERSION strings (PR #24); add doctor SHA-vs-HEAD + pre-JSON-prompt drift check.

**Surface cluster (cross-cutting Rule 1–5 + Rule 6 prose).**
- Register `sworn://baton/rules` MCP resource (or fix the init AGENTS.md pointer); doctor check that every advertised URI resolves.
- Rule-2 dark-code/deferral lint; promote `open_deferrals` to a why+tracking+ack struct.
- Rule-4 commit-body surfacing at the auto-commit moment.
- Decide/remove dead `adopt.Materialise`/`SpliceAgents`.
- EARS single matcher; DoR on planned edge; unify the two RTM parsers; designaudit diff-scope + test-file exclusion + flag re-sync.

**Rule-10 journeys to declare (ratified `.sworn/journeys.json`).**
- **J — keyless-full-loop:** run the full plan→implement→verify→merge loop keyless on a subscription (crosses the entitlement/credits boundary; requires entitlement-boundary no-mock detection + a real-infra attestation before ship).
- **J — loop-verifier negative scenario:** an implemented-but-thin slice MUST NOT reach `verified` through the autonomous loop without a fresh-context certify — the Rule-7 negative path, walked end-to-end against real infra.
- Plus the elicited CLI journeys (onboard/init, develop-feature) once elicitation is model-driven.

---

## 5. What is genuinely full (credit where due)

Sworn is at fidelity on a real, load-bearing core — this is not a hollow shell:

- **Rule 5 Session Discipline** — text at fidelity across three copies with working adopt/doctor wiring; the docs/release re-anchoring is an in-band documented exception.
- **Rule 9 stakes classification** — Type-1/2 state types match canonical field-for-field; the designfit gate fails closed on both Type-1-without-decision and arch-significant-but-Type-2, wired and tested; Layer-1 design checks + Layer-2 human cohesion verdict both correct and fail-closed; design-allowlist auto-read; ui_bearing fail-closed; Captain Step 2b a byte-identical port.
- **Rule 11 scoped git mutation** — every git op scoped via cmd.Dir, never os.Chdir; single chokepoint fails closed on empty Dir; a real reachability test proves the guard fires AND the ambient repo is unmutated. This is the model implementation.
- **Orchestration deterministic core** — the git-ref **oracle reader** (the prior audit's P0-missing keystone) and the pure-function **slice router** are faithful, tested, and live-wired; the in-loop stateless judge's payload genuinely excludes the implementer transcript (spec-conformant context separation).
- **Implementer role** — driven by the real Baton prompt, honours the implemented checkpoint, never self-certifies (FSM-locked).
- **Captain design-review** — built and engine-wired (refuting the seed claim that `prompt.Captain()` is never called by non-MCP code).
- **Model Error{Kind} taxonomy (production)** — typed kinds, HTTP classification, JSON message lift, errors.Is/As, routed through eight drivers.
- **Rule 8 Coverage gate** — runs, maps every AC, fails closed.
- **Rule 10 Impact analysis** — fail-closed, derived from touchpoints, with a deliberate over-inclusion bias and acknowledged blind spot.
- **doctor embed-integrity** — presence/length/headings checked and fail-closed (well-formedness, not currency).
