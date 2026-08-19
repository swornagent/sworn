# Issue audit and release portfolio — 2026-08-20

Audited all 50 open issues against `release/v1.0.0 @ 4e1ee1fa` (post
record-economics). 21 closed with evidence comments: 12 delivered
(#157 #170 #181 #187 #190 #199 #201 #203 #211 #213 #215 #216), 3
superseded/consolidated (#152 #160 #179→#220), 6 stale (#122 #123 #124
#125 #126 #182). 29 remain. This capture slices the survivors into
releases by capability seam: a release groups interdependent outcomes;
each slice owns one seam (one mechanism, per the planner doctrine).

Ordering rule: R1 is brought forward deliberately — the operator
surface is what makes Sworn usable for daily real work, which is the
adoption path and the dogfood engine for everything after it. R2 is
time-sensitive in the only real sense: telemetry not captured is lost
forever. R3 protects the unattended story record-economics earned.

## R1 — operator-surface (brought forward)

Covers #177(S5b) #196 #172 #192 #193 #173 #169(tail).

- **S1 project-discovery** (`cmd/sworn`): one shared discovery seam
  over `.sworn/runs/*.journal` + `release-wt/*` refs powering bare
  `sworn run`, `sworn run <release>` (three-step precedence per #177),
  and serve-without-flags. The TUI's private discovery
  (`tui_project.go`) refactors onto it. Anchor: #177's S5b text.
- **S2 detached drive** (`internal/runtime` service): the single
  mechanism under both #172 and #192 — `Start`/`AnswerAttention`/
  `Control` return once the command is durable; the drive continues on
  a background owner. `answer | head` can no longer orphan a run.
  Anchors: service.go driveOwned call sites (1083-1100, 1151, 1176).
- **S3 control-verb truthfulness** (`internal/runtime` +
  `internal/cockpit` presentation): #193 — takeover during an
  unexpired lease returns a named wait signal (extend the existing
  Resume branch), and the board's "takeover required" hint derives
  from the actual takeover admission conditions. Anchor:
  service.go:1137-1146 switch.
- **S4 global serve** (`internal/cockpit` + `cmd/sworn/operator.go`):
  #196 ruling 3 — `sworn serve` with no flags serves the project:
  release/run selection, lifecycle states, a needs-you row. Consumes
  S1, which also locates `.sworn/operator.json` by convention (today
  the operator config is flag-only and `sworn init` does not scaffold
  it). Anchor: operator.go:128-137 flag gate.
- **S5 event association** (`internal/cockpit` projector): #173 —
  `Evidence` gains effect/work/track association so the feed filters
  per track; TUI backend contract gains the paged feed. Anchor:
  cockpit/model.go:169-173, projector.Events (exists, unexposed).
- **S6 TUI reconnect + honest diagnostics** (`internal/tui`): attach
  to a detached run (consumes S2), keys live during execution, #169
  tail (explanations for PLAN_NOT_FOUND / INVALID_PLAN_FENCE /
  REF_NOT_FOUND / INVALID_HEAD_OBJECT; the no-manifest message names
  the searched location; control lockout keyed off truth not
  staleness). Anchors: tui/model.go:113,140,340; view.go:481-501.
  RULING 2026-08-20: key config is surfaceable in the TUI where it
  makes sense — read-first: the resolved role/profile/model matrix from
  drivers.json, operator config (listen, telemetry endpoints,
  webhooks), records root, journal/manifest paths. Same resolution
  seam as S1/S7; editing from the TUI is a later decision, visibility
  is not.

- **S7 guided init** (`cmd/sworn` init): RULING 2026-08-20 — `sworn
  init` is the guided per-project setup, not just scaffolding: it walks
  the operator through drivers (agent detection exists), the operator
  config (listen, telemetry endpoints when R2 lands, webhooks), and
  ends with the next command to run. Idempotent on re-run; `--yes`
  keeps the non-interactive path. Composes with the ratified home-
  surface and --task quickstart directions.

Deferred out of R1: #195 (tool-result stream) and #176 (live output)
— the deep observability seam is R6; the feed association in S5 is
useful without it.

## R2 — telemetry-foundations

Covers #209, #201-rider, the two-channel export design, #205's
notification-kind sliver. Full spec below.

- **S1 usage truth** (`internal/driver` usage propagation →
  `internal/runtime` dispatch → `internal/observe`): non-null token
  capture per provider surface; `token_status=unavailable` becomes
  loud (journaled reason + eval summary names the gap instead of
  nil-ing the aggregate at eval.go:544-546); per-turn capture; turn
  economics counters (turns, tool calls per turn, call mix per role).
- **S2 semconv spans** (`internal/observe` otel): dispatch spans carry
  GenAI semantic conventions — `gen_ai.request.model` (real model id),
  `gen_ai.usage.input_tokens` / `output_tokens` + cache/reasoning
  splits, duration; role/slice/release as sworn.* attributes. Evidence
  anchor: a Langfuse instance receiving the stream renders
  per-dispatch generations with model, token split, and timing.
- **S3 two-channel export** (`internal/observe` config + `cmd/sworn`
  wiring, extending R1-S7's guided init with the telemetry step (private endpoint, share opt-in) — export must not require reading the source; and
  note export is serve-mediated today, so a run with no cockpit
  attached emits nothing, which is share-channel data loss): `telemetry.private` (today's config: endpoint+headers,
  operator's own) and `telemetry.share` (opt-in, default endpoint =
  the project's collection gateway, overridable/disableable). The
  share channel passes a schema allowlist enforced in-engine: named
  metrics/attributes only, structurally no prompts/responses/
  reasoning/ids/content. Both are plain OTLP/HTTP.
- **S4 notification fidelity** (`internal/cockpit` webhook):
  `degradation_budget_parked` and friends map to named notification
  kinds instead of the coarse `run_updated` (webhook.go:806).

## R3 — unattended-operability

Covers #221 #207 #191 #205(guards) #204(remainder); #224 joins once
its ruling lands. Candidate small additions on the same seams: #188
(Bash `command` alias — driver toolset), #189 (.git mask as empty
regular file + prompt note — driver containment), #217 remainder
(quota body surfaced instead of cleared at http.go:224; pacing cap
wired beyond gemini), #194 (factory temp-root reaper).

- **S1 credential preflight** (`internal/driver` native): #221 —
  expiresAt read at dispatch preparation, refuse `CREDENTIAL_STALE`
  before burning tries; native auth-class errors distinguished from
  transport.
- **S2 claimed-dispatch recovery** (`internal/runtime` dispatch +
  `internal/journal` control): #207 — the checkpoint claimed-case
  reconciles instead of RECOVERY_UNCERTAIN; takeover/retry gates and
  the board hint agree with each other.
- **S3 honest-yield parking** (`internal/runtime` turn recovery):
  #191 scoped to honest `question`/`blocked` yields — park on first
  occurrence with the question surfaced; the recovery-budget "many
  nudges" doctrine stays for operational nudging.
- **S4 economy guards** (`internal/runtime` reducer + manifest):
  #205 guards 2+3 — per-work turn/output-token budget; N consecutive
  identical error codes parks early. Guard 2 is the one that would
  have caught the ~900-turn slice.
- **S5 continuation lifetime + yield×replay** (`internal/driver`
  continuation + `internal/runtime`): #204 remainder — manifest
  control over the 24h lifetime; the INVALID_CONTINUATION-during-
  yield × opaque_replay.reuse interaction fixed and pinned.

## R4 — contract-identity

Covers #200 #210 #218(remainder). Root cause is shared: contract and
receipt identity is path- and whole-bytes-shaped.

- **S1 digest-addressed contracts** (`internal/baton` plan/record +
  `internal/runtime` install): resolve slice contracts by canonical
  digest, path becomes a hint; revisions stop churning paths.
- **S2 proposal contract persistence** (`internal/runtime` proposal):
  #210 — proposals carry contract bytes durably; the engine computes
  canonical digests at proposal time (no hand arithmetic).
- **S3 receipt identity split** (`internal/baton` receipts):
  #218 — acceptance/scope identity vs checks identity, so a checks
  edit stops voiding design receipts.
- **S4 engine-bindings role-asset addendum** (assets + digest chain):
  the three sentences (canonical digests; before/product_tree are
  invocation-state digests; seal epoch lockstep) that cost review
  turns in both instrumented releases.

## R5 — continuation-economics

- **S1 verifier resume + delta injection** (`internal/runtime` turn
  recovery + `internal/driver`): #219 — a FAIL no longer disqualifies
  the retained verifier continuation; remediation re-verification gets
  the candidate delta injected and journaled; degradation labelling
  says which channel a re-verification used.

## Later ladder

- **R6 worker-observability**: #195 (tool-result emit seam to journal/
  cockpit) + #176 (provider-neutral live output, bounded transcript
  store). Large; transforms supervision.
- **R7 navigation**: #202 LSP-mux (ratified; unblocked now that
  batching landed).
- **R8 test-economics**: #208 (journey-prefix snapshots, race
  scoping) + #151 (release-notes link transform).
- **Watching**: #223 (hardening landed; open until the markers catch a
  live hang), #220 (pin policy; act when the next CLI bump forces it),
  #224 (awaiting the park-vs-carve-out ruling).

## Telemetry-foundations: spec detail

Design rulings this spec encodes:
1. Two channels, one wire format. Both are OTLP/HTTP with
   endpoint+headers. `private` = operator's own verbose stream,
   points wherever the operator says (Brad: same collector as share).
   `share` = opt-in fleet stream, defaults to the project gateway,
   carries only the versioned share schema.
2. The share schema is enforced in-engine (allowlist at the exporter
   boundary), then again at the gateway collector. The privacy page's
   promise (no prompts, responses, reasoning, session ids, source
   content) becomes structural.
3. Spans adopt OTel GenAI semconv so any backend renders generations
   natively; sworn-specific facts ride as `sworn.*` attributes
   (role, slice, release, verdict, attempt, epoch/try).
4. Verdicts map to scores at the backend (Langfuse scores API is the
   first consumer) — but the engine only emits attributes; mapping is
   backend-side, keeping the engine vendor-neutral.
5. Backend topology (operator-side, not engine scope): an OTel
   Collector gateway is the fixed point — auth, tenancy tagging,
   share-schema double-enforcement — fanning out to ClickHouse (system
   of record), Langfuse (LLM lens), Prometheus/Grafana (economics).
   The engine never knows any of this; it sees two OTLP endpoints.

Acceptance sketches (final contracts to be authored at release time
under the doctrine, one mechanism per slice, anchors as evidence):
- S1-A: no dispatch on any certified surface journals null tokens
  silently — either real usage or a loud journaled
  `token_status=unavailable` with the surface named; the eval summary
  reports coverage instead of nil-ing (anchor: eval fixtures +
  per-surface usage tests).
- S2-A: a fixture OTLP sink receives dispatch spans carrying
  gen_ai.request.model / usage tokens / duration; span attribute
  goldens pin the schema (anchor: observe otel tests).
- S3-A: a share-channel export with a prompt-shaped attribute is
  structurally impossible (allowlist test); private and share channels
  configure independently; share default endpoint is overridable and
  absent-by-default until opted in (anchor: observe config tests).
- S4-A: a degradation park produces a webhook notification whose kind
  names the park reason (anchor: webhook projection test).
