# Baton Conformance Audit — SwornAgent (2026-07-02)

**Supersedes** `docs/captures/2026-06-27-baton-conformance-audit.md` (see the
banner added to that file). Where the two disagree, this one is current: it is
built from live repo state at `release/v0.1.0` @ `632d4f3`, the June one is not.

**Scope:** Does SwornAgent implement the Baton rule-set (Rules 1–11),
records-as-JSON, the orchestration engine, role ontology, and the model/provider
layer against live state?
**Method:** 11 dimension finders, each running live commands (`sworn doctor`,
`lint trace`, `lint ac`, `reqverify`, `designfit`, `designaudit`, `journeys
--check`, `ship`, raw MCP JSON-RPC, `go test`) in disposable worktrees, cross-
referenced against the open-issue backlog. **Every** finding was then re-checked
by an independent fresh-context adversarial verifier that reproduced the evidence
itself. 71 findings: **56 CONFIRMED / 14 ADJUSTED / 1 REFUTED**; **102** elements
confirmed conformant. No paid model was dispatched (bogus/absent keys used to
probe wiring); the audit ran against committed state only.

---

## 1. Verdict

**Sworn is now substantially closer to full Baton conformance than the June 27
audit found, and the gap has changed shape.** Every one of June's five headline
CRITICAL structural gaps has been closed or materially reduced by work that
landed on `release/v0.1.0` since:

| June 27 headline gap | Live state 2026-07-02 |
|---|---|
| Records-as-JSON unbuilt; `example.com` `$schema`; no validation | **Built.** 7 schemas vendored with canonical `$id`; a real draft-2020-12 evaluator (`ValidateSchema`) exists and validates on write for status/spec/proof/journeys/attestations; live emission stamps canonical URLs — `example.com` gone from the emit path. |
| `board.json` missing; oracle parses `index.md` frontmatter | **Cutover executed.** `board.json` object/strict shape is live on 4 of 5 releases; the S05 strict reader fails closed on legacy bare-string; `index.md` is rendered *from* `board.json`, never the reverse. |
| Proof bundle hand-templated, gate optional | **Emitted + gated.** `proof.json` (proof-v1) is emitted and validated on write; the engine cannot reach `implemented` without both `proof.md` and `proof.json`; the proof-mandatory gate fires before verifier dispatch and fails closed. |
| Loop verifier not Rule-7-grade (stateless tool-less judge) | **Keystone landed.** ADR-0011 step 3 merged: the prose verdict scraper is deleted from the hot path; the engine dispatches the real `verifier.md` role prompt and consumes a schema-validated `verifier-verdict-v1`. |
| Rule-10 journey gate opt-in only; no-mock blind | **Wired.** Ratified `.sworn/journeys.json` exists; `journeys --check` and the `ship`/cutover gate have real teeth; the gate is wired into the CLI `merge-release` path; the no-mock detector now carries entitlement/subscription keywords. |

Also newly full since June: the **Orchestrator role is formalized** (was
"unnamed deterministic code"); **terminal `Error{Kind}` handling** on the
implementer path; **EARS no longer rejects the spec's own examples**; the **RTM
chain fires on real intake** (June's `needs:0` vacuous-pass trap is closed on
spec.md-era releases); Baton **VERSION strings are consistent** (June's three-way
split resolved).

**Where the gap moved.** The dominant conformance risk is no longer "the layer
doesn't exist" — it is **format lag and unwired gates**: a family of Rule-8/9
gates and the parallel engine still read the *old* `spec.md`/frontmatter surface
and so silently no-op or hard-error on the very `spec.json`/`board.json` records
Sworn now produces. Three of the four live CRITICALs are exactly this:

1. **The parallel loop and MCP merge gate are dead on canonical (rendered)
   boards** — they read tracks from `index.md` frontmatter, which post-cutover
   carries only title/description. Fail-closed (exit 1 / not-found), not
   false-ready — but the engine cannot run its own current record shape. This is
   precisely what the in-flight release `2026-07-01-render-drift-reconciliation`
   exists to fix (its entire intake), so it is **already tracked** there, not a
   new issue.
2. **`sworn reqverify` vacuously passes (exit 0) on `spec.json`-only releases** —
   extracted 0 ACs from the absent `spec.md` and never dispatched. *(Fixed this
   pass.)*
3. **`sworn doctor --fix` destroyed AGENTS.md, clobbered its own backup, never
   converged** (`sworn#43`). *(Fixed this pass.)*

The fourth live CRITICAL — **`ProductionMergeTrack` silently no-ops on every
real release worktree** because it tests `.git` as a directory when a
`git worktree`'s `.git` is a *file*, then `finishTrack` logs "auto-merged" and
returns TrackPass — is a fail-*open* Rule 11 target assertion (the dangerous
kind). *(Fixed this pass.)*

---

## 2. Finding totals

70 real findings (excludes the 1 REFUTED): **4 critical, 21 high, 28 medium,
17 low.** By backlog relation: **46 new, 20 match an open issue, 4 were open
issues already fixed in live state** (closed this pass).

---

## 3. Conformance map by dimension

Legend: **full** = live-verified conformant; **partial** = works on one surface/
format but not the record shape Sworn now emits; **gap** = confirmed defect.

### Rules 1–5 (advisory text + surfacing)
- **full:** all 11 rule docs vendored + integrity-checked; Rule 5 surfacing via
  adopt/doctor; D6 strict-additive `open_deferrals` carrier; registered MCP baton
  resources resolve.
- **gap (fixed this pass):** `sworn://baton/rules` was unregistered + `resources/
  list` hardcoded empty (dead link the init template advertises); `doctor --fix`
  AGENTS.md destruction (`sworn#43`); `init` couldn't create AGENTS.md cold-start
  (`sworn#28`).
- **gap (tracked):** no deterministic Rule-2 deferral gate (`sworn#25`); Rule-4
  engine auto-commits are single-line, no decision body (`sworn#72`); advisory
  fragment drifted to the seven-rule era (`sworn#73`).

### Rule 6 — Proof Bundle
- **full:** `proof.json` emitted + validated on write; `implemented` unreachable
  without proof.md + proof.json; proof-mandatory gate fires before verifier
  dispatch, fail-closed; proof.md sections generated from live state (no more
  hardcoded `None`); proof-v1 `$id` matches canonical and is stamped on emit.
- **gap (tracked):** no gate ever *reads/validates* `proof.json` — a
  merged+verified release carries proofs that fail Sworn's own validators, one
  slice verified with no proof.json, two with no proof.md (`sworn#54`); proof.md
  reachability still boilerplate, Delivered overclaims all ACs, test-command
  hardcodes the full suite (`sworn#62`); vendored proof-v1 schema is a dialect
  fork under the canonical `$id` (`sworn#48`).

### Rule 7 — Adversarial Verification
- **full:** prose scraping deleted from the hot path (keystone step 3);
  `verifier-verdict-v1` byte-identical to canonical and genuinely consumed,
  fail-closed; engine dispatches the real `verifier.md`; implemented-checkpoint
  and fail-closed FSM honored; BLOCKED path satisfies the machine-readable
  violations contract; `verifier_was_fresh_context` set with a defensible
  payload-level basis on the engine path.
- **gap (tracked):** the in-loop agentic verifier is still a **single-shot,
  no-tool** ChatStructured call — it structurally cannot re-run tests or read the
  live repo that `verifier.md` mandates (`sworn#55`); terminal verifier dispatch
  errors mapped to INCONCLUSIVE and walked the paid escalation ladder *(fixed
  this pass)*; INCONCLUSIVE-vs-FAIL recovery still conflated (`sworn#61`); FAIL
  drops typed violations, verifier model id unconfirmed, `fresh_context` hardcoded
  false on the `route` JSON (`sworn#60`).

### Rule 8 — Requirements Fidelity
- **full:** needs-parsing + full RTM chain fire on spec.md-era content; EARS
  classifier no longer rejects everything; reqvalidate is format-agnostic +
  fail-closed; DoR gate exists and composes the three legs; spec/proof validated
  on write; spec-v1 records carry high-quality structured ACs.
- **gap (fixed this pass):** `reqverify` vacuous-pass on spec-v1 *(fixed)*; `lint
  trace` AC-level legs silently skip without spec.md, so 2 of 3 RTM legs inert on
  current-format releases *(fixed)*.
- **gap (tracked):** `lint ac` hard-errors (exit 2) on spec-v1 (`sworn#56` covers
  the DoR/rtm hard-error family; lint-ac tracked under the `sworn#22` scraper
  family); DoR unreachable on the engine cold-start path + hard-errors when
  reached (`sworn#56`); merged+verified release carried **zero** validation
  records (`sworn#57`); five divergent spec.md scrapers (`sworn#22`); spec-v1
  schema drops the Rule-8 thread constraints (`sworn#48`).

### Rule 9 — Design Fidelity
- **full:** designfit fail-closed Type-1 gate (genuine non-vacuous PASS on a real
  release); designaudit Layer-2 human cohesion verdict fail-closed; ui_bearing-
  without-design-system fails closed; Captain design-review engine-wired on the
  planned edge (happy path); redesign loop closes.
- **gap (fixed this pass):** router `review` decision killed the whole track
  (`sworn#46`) *(fixed)*; the design gate silently proceeded on a
  generation/dispatch error with no Rule-2 deferral *(fixed — now records a
  machine-readable deferral / halts)*; the engine dispatched the **conflated
  captain.md** instead of the delivered `design-reviewer.md` (S19 split had
  regressed to dark code) *(fixed — split files embedded + dispatched)*.
- **gap (tracked):** `designaudit` diverges from the canonical Layer-1 contract on
  all five documented properties (whole-tree not diff-scoped, token-blind, no
  allowlist file, scans test files, reads global config) (`sworn#58`); designfit
  wired into no workflow edge + fallback hardcodes Sworn-only path prefixes
  (`sworn#59`).

### Rule 10 — Customer Journey
- **full:** ratified durable journeys artefact; `journeys --check` fail-closed;
  ship/cutover gate has real teeth; gate wired into CLI `merge-release`; impact
  analysis fail-closed + over-inclusive; no-mock detector carries entitlement
  keywords; attestation completeness enforced human-first; artefact writes schema-
  validated.
- **gap (tracked):** the **engine** merge path routes to the Driver-1
  `/merge-release` slash command, which runs no journey step — so the gate binds
  on the CLI surface but not the autonomous loop (`sworn#53`); ship gate passes
  vacuously when the impact heuristic matches zero journeys (`sworn#76`);
  elicitation is a static scaffold, not model-drafted (`sworn#77`); emitted
  journeys/attestations shapes diverge from canonical v0.7.0 (`sworn#48`).

### Rule 11 — Process-Global Mutation Guard
- **full:** `git.Repo` empty-Dir chokepoint guard (`sworn#6` root cause, closed);
  CLI + MCP merge-track target assertions; no `os.Chdir` in production; designaudit
  config mutation is save/restore incl. unset; LoadDotEnv bounded; RunSlice +
  parallel-bootstrap git ops directory-scoped.
- **gap (fixed this pass):** `ProductionMergeTrack` fail-*open* `.git`-is-dir
  guard silently skipping every real worktree merge *(fixed — accepts file-or-dir,
  errors on non-worktree)*.
- **gap (tracked):** no deterministic Rule-11 detector for the slices Sworn builds
  (`sworn#64`); gate diff helpers run `git diff` in ambient cwd + finishTrack
  swallows `git push` failure (`sworn#63`); pre-existing track worktree used
  without identity assertion + can't resume after removal (`sworn#65`).

### Records-as-JSON / schema fidelity
- **full:** verifier-verdict-v1 byte-identical to canonical + emitted via the real
  evaluator; all 7 vendored schemas declare canonical `$id`; live emit stamps real
  `$schema`; board.json cutover live (4/5); S05 strict reader fails closed;
  validate-then-write ordering correct; Baton-side spec/proof/journeys schemas
  published at v0.7.0 (`baton#46/47/48` closed).
- **gap (tracked):** 6 of 7 vendored schemas **content-diverge** from published
  v0.7.0 under identical `$id` (the exact class this project has repeatedly
  tripped) — scoped for the re-vendor (`sworn#48`); every writer still validates
  via the legacy top-level-only `baton.Validate`, not `ValidateSchema`
  (`sworn#39`); read side is fail-open — a corrupted `state` enum flows through all
  readers undetected (`sworn#52`); `WriteBoard` validates *after* writing to disk
  (`sworn#66`); dead `example.com` residue in code + on disk (`sworn#67`); legacy
  string board unmigrated (`sworn#44`) + oracle legacy fallback bypasses strict
  validation (`sworn#42`).

### Orchestration engine
- **full:** merge gate reads committed state through the oracle; oracle prefers
  board.json + fails closed on ref skew; durable SQLite decision log; PID-liveness
  crash recovery; breaker/max-turns/INCONCLUSIVE all *pause* not fail; keystone
  interpreter landed; observed parallel-loop failure modes are fail-closed;
  `run --task` planner front-half is real.
- **gap (fixed this pass):** router `review` → TrackFail (`sworn#46`).
- **gap (tracked):** engine dead on canonical boards (**critical**, tracked by the
  in-flight `2026-07-01-render-drift-reconciliation` release, not a GH issue);
  `run --task` implement+verify handoff broken (`sworn#27`); PauseEngine
  unreachable dark code (`sworn#68`); proxy routing overrides an explicit provider
  key (`sworn#69`); track-worktree resume gaps (`sworn#65`).

### Role ontology + vendor/contract
- **full:** vendored role prompts are records-as-JSON era (== baton v0.6.3);
  verifier.md designaudit invocation matches the binary (June flag-mismatch
  closed); one authoritative consistent VERSION pin; Orchestrator role formalized.
- **gap (fixed this pass):** captain/design-reviewer split regressed to dark code
  *(fixed)*.
- **gap (tracked):** re-vendor to v0.7.0 (`sworn#48`); `internal/prompt/VERSION.txt`
  dead provenance (`sworn#74`); doctor pin-currency is a layout heuristic, reports
  OK one release behind (`sworn#75`).

### Model / provider layer
- **full:** Chat driver coverage expanded; capability gate fail-fast not mid-run;
  terminal `Error{Kind}` on the implementer path; `sworn#32` reasoning_content
  fallback present; sonnet-5 introductory pricing present; typed error taxonomy on
  the wire; codex deferral Rule-2 compliant; anthropic-sdk ADR-justified; dispatch
  telemetry records the actual model.
- **gap (fixed this pass):** agentic-verifier terminal errors → INCONCLUSIVE
  *(fixed)*.
- **gap (tracked):** Anthropic + claude-cli advertise CapChat but ignore tools —
  implementer gate passes drivers that can't tool-call (`sworn#35`); cost telemetry
  still nominal flat $2/1M, pricing registry dark (`sworn#70`); reasoning fallback
  untested (`sworn#71`); factory still a hardcoded switch (`sworn#15`); pricing
  triplicated (`sworn#41`).

---

## 4. What was fixed this pass (11 slices, each certified)

All on branch `audit/2026-07-02-conformance-gap-closure` (5 sub-branches merged;
full `go test ./...` green, 41 packages). Each fix landed as its own commit with a
proof bundle at `docs/captures/2026-07-02-fix-<id>-proof.md`, verified RED-at-base
/ GREEN-at-fix by an independent fresh-context session.

| Fix | Sev | Commit | Issue |
|---|---|---|---|
| `ProductionMergeTrack` fail-closed on non-worktree target (.git file OR dir) | critical | 923e49c | new |
| `doctor --fix` splices AGENTS.md (idempotent, backup-safe, no dead path) | critical | 08abd21 | `sworn#43` |
| `reqverify` reads spec.json ACs, fails closed on zero evaluable ACs | critical | cea9c46 | new |
| router `review` decision pauses the track (not TrackFail) | high | 6d215ba | `sworn#46` |
| design gate records a Rule-2 deferral / halts instead of silently proceeding | high | 2e761c0 | new |
| agentic-verifier terminal dispatch errors → BLOCKED, not INCONCLUSIVE | high | e5e1eff | new |
| `sworn verify` fails closed on empty spec + missing/empty/malformed proof | medium | 05b292a | new |
| `sworn://baton/rules` registered + `resources/list` enumerated | high | 4121e26 | new |
| `init` embeds templates → cold-start AGENTS.md creation | high | 9cc6f9b | `sworn#28` |
| design review dispatches `design-reviewer.md`, not conflated captain.md | high | 8f4bb1f | new |
| `lint trace` evaluates AC legs from spec.json on spec-v1 releases | high | 9bd6456 | new |

---

## 5. Issue backlog reconciliation

- **Closed — already fixed in live state (stale-open):** `sworn#6`, `#30`, `#33`;
  `baton#46`, `#47`, `#48`. (6)
- **Closed — fixed this pass:** `sworn#43`, `#28`, `#46`. (3)
- **New issues filed** from confirmed findings: `sworn#52`–`#77`. (26)
- **Re-confirmed with fresh evidence on the existing open issue:** `sworn#10, 15,
  22, 25, 27, 35, 39, 41, 42, 44, 48`. (11)
- **Umbrella / session anchor:** `sworn#51`.

No confirmed finding was left untracked. The one live CRITICAL not fixed here
(engine-dead-on-canonical-boards) is owned by the in-flight
`2026-07-01-render-drift-reconciliation` release, whose intake is that exact gap.

---

## 6. Honestly still unknown / untestable in this pass

The no-paid-dispatch constraint means every **end-to-end model-driven path** was
verified by wiring + code-read + deterministic tests, never by a real run:

- A real `sworn loop` / `--parallel` implement→verify→merge on live content
  (all dispatch paths). The fail-closed *wiring* was proven with bogus keys; the
  *behaviour under a real model* was not.
- **Bogus-key probing is not fully reliable on this host:** logged-in `~/.sworn`
  credentials can route dispatch through the SwornAgent proxy, so a "no key" probe
  may not actually be keyless. Findings that depend on this were downgraded to
  code-read confidence where noted.
- Verifier-verdict-v1 emit→validate round-trip over a real model; whether a real
  model given the worktree-oriented `verifier.md` in a no-tool single-shot call
  reliably emits BLOCKED.
- Whether `CheckDoR` has *ever* fired in a real engine run.
- MCP write paths (`create_slice`) and `approve_merge` live behaviour through a
  real client (only raw JSON-RPC over stdio was exercised).
- Whether the private `~/.claude` slash-command harness compensates for unwired
  gates (designfit, journey-on-merge) at merge time — outside this repo.
- Whether `sworn#40`'s cutover of the 127 coach deferrals happened (data lives in
  private coach state).
- Whether the `sworn#48` re-vendor will regress test fixtures in other packages
  (memory warns the S05 strict-reader change did) — not attempted here.
- The 4 uncommitted `internal/mcp/*.go` changes in the primary checkout were
  **excluded** from the audit (it ran against committed `632d4f3`); their effect
  on the MCP findings is unassessed.

---

## 7. What is genuinely full (credit where due)

The load-bearing core is real and, since June, materially more complete: the
git-ref oracle + board.json strict reader; the records-as-JSON layer with a real
draft-2020-12 evaluator validating on write; the keystone (prose scraper deleted,
verifier emits schema-validated verdicts inside the loop); proof.json emitted and
gated before `implemented`; the journey gate with teeth on the CLI merge path;
scoped git mutation with a fail-closed empty-Dir chokepoint; a formalized
Orchestrator; a durable SQLite decision log and PID-liveness crash recovery; and a
typed provider-error taxonomy with terminal-error handling on the implementer
path. The remaining work is concentrated and legible: **make the Rule-8/9 gates
and the parallel engine read the `spec.json`/`board.json` records Sworn already
produces, wire the gates that exist into the edges that should trigger them, and
re-vendor the schemas/prompts to v0.7.0.**
