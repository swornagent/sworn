# Run report — 2026-08-23-telemetry-foundations

Nine slices, one track, five runs, five plan revisions, four captain
escalations, twelve verifier passes including assembly, zero candidate
rejections. Merged at 89da83a6 roughly thirty-three hours after the
contracts were approved. This is the release that makes the run own its
facts — usage truth, GenAI spans, run-side export, honest parks,
surviving provider evidence, persistent failure bodies, a streaming
implementer, journaled tool results, and the restored dispatch
responsibility (sworn#229, ruled by approval of S9).

## Delivered

| Slice | Outcome | Candidate |
|---|---|---|
| S1 usage-truth | full token split incl. reasoning, duration, cost, model id; loud unavailable; coverage in-band; turn economics | 9c759de0 |
| S2 genai-spans | gen_ai.* dispatch spans, one run = one trace, verdict absence pinned | 7ef01c56 |
| S3 runside-export | engine exports its own OTel from every delivery-hosting process; private + share channels; in-engine share allowlist; guided-init telemetry step | fe50e4a5 |
| S4 degradation-truth | budget counts real loss only; parks name cause, count, budget, knob on status, board, webhook | 484acc2e |
| S5 provider-limit-evidence | provider's words survive to journal and stream; hard walls fail fast instead of pacing for hours | (r5) |
| S6 failed-observation-persistence | failed attempts keep digest-verified readable observation bodies | 7a98d227 |
| S7 implementer-stream | google-native streams; thought summaries requestable; pacing parity pinned | (r5, t2 after a corrupted t1) |
| S8 tool-result-observation | every tool result lands bounded, redacted, identity-carrying in the journal | b3995ac8 |
| S9 dispatch-responsibility | dispatch event bodies name the responsibility again; e2e reads it directly | f5339b2f |

## The revision ledger — four escalations, four real gaps

Every plan revision was triggered by an opus captain escalation, and
every escalation was a genuine contract defect, not churn:

1. **rev2** — S1's A4 schema bump needs the eval schema-version
   allowlist in internal/journal/observer.go; rev1 scoped journal out.
2. **rev3** — the same bump invalidates two e2e assertions pinning
   EvalSchemaVersionV2; an operator tree sweep proved these plus the
   allowlist were the only pins of the class, closing it in one move.
3. **rev4** — S2's sworn.verdict attribute was unbindable: no
   per-dispatch verdict exists on any read surface, and the slice-level
   last-known-wins outcome would state a false fact on a historical
   attempt span. A3 now pins the attribute's absence.
4. **rev5** — the S3 designer proposed minting a share-channel default
   egress hostname; the captain correctly refused an authority it did
   not hold. The revision names the human-ruled gateway
   (https://otel.sworn.sh, 2026-08-20 infra ruling), inert until
   explicit share opt-in.

Every revision was operator-carried: the in-run replan path is
structurally unusable headless (see below). Receipts adopted by
ancestry across all five runs; no verified work was ever re-run.

## The walls, and what they taught

**Exact-bytes-by-regurgitation does not scale.** The r1 planner had to
reproduce a 20.5KB plan as ~27KB of inline base64 through provider
function-call transport; thirteen INVALID_EXACT_BYTES later it found
the undocumented path form and succeeded. The path machinery existed,
worked, and had zero tests and zero documentation — the Service.Wait
defect class again (built affordance, no caller, no discoverability).
S8's merged code fixed the root cause: the sworn_submit tool
description now teaches the path form up front, and by r5 workers were
using it unprompted for check evidence.

**The in-run replan flow is headless-incompatible.** A production
planner may emit plan bytes only from a dispatch resumed after an
answered human turn (summary-before-plan,
validateHumanConfirmedPlannerHandoff). Every post-escalation planner
dispatch therefore burned its tries and parked — six failed dispatches
across r1-r3 before the operator monitor learned to kill the run at the
escalate boundary. Worse, a sealed planner proposal's plan bytes are
durable nowhere when the dispatch fails run-side: the sworn#195
postscript incident reproduced exactly, and revision 2 had to be
reconstructed from streamed logs and canonical-digest verification.

**Provider transport is a fingerprint, not a standard.** deepseek-v4-pro
leaked its internal DSML tool-call markup into arguments three times
(each correctly refused, each costing a dispatch retry).
OpenRouter's chat endpoint is unusable by the strict chat decoder (it
injects a root "provider" field), its responses endpoint decorates
usage with cost fields, and its SSE is data-only with no event: lines
— three tolerance gaps found while certifying stealth/ox-alpha, two of
which required operator engine patches that must now land properly.

**Provider evidence pays for itself immediately.** Mid-release, a
26-deep PROVIDER_LIMITED storm was diagnosed benign in one direct curl
(deepseek minute-window pacing, not a wall) — the manual ritual S5 was
at that moment being built to retire.

## Roster verdicts

- **claude-opus-5, captain**: the star of the release. Four
  escalations, all load-bearing; independently reproduced canonical
  digests to dissolve an implementer's false alarm; refused to
  authorize an egress default outside its remit; adjudicated design
  questions so they never recurred. One transient ADAPTER_FAILURE and
  one credential-rotation kill, both absorbed by retries.
- **claude-sonnet-5, verifier**: twelve for twelve including assembly,
  no rejected candidate, re-executed declared checks itself.
- **deepseek-v4-pro, implementer**: strong candidates (every one
  passed verification first attempt), disciplined self-correction, but
  three DSML tool-call corruptions and heavy minute-window pacing on
  ~290K-token contexts.
- **deepseek-v4-flash, planner/recovery**: competent under an
  impossible transport constraint; its systematic probing of the
  submit surface (tiny-payload controls, digest bisection) was the
  diagnostic that exposed the exact-bytes wall.

## Self-instrumentation

S3 went live mid-release: the back half of r5 exported its own spans
run-side with no serve process, through the machinery the release was
built to deliver. The first workload observed end-to-end by
telemetry-foundations was telemetry-foundations.

## Carried forward

Operator patches at the merged head (ops/operator-engine-patches.diff,
in the ops binary, to be landed as a proper slice): responsesUsage
tolerates OpenRouter's cost decorations; readStreamedResponse falls
back to the payload's self-describing type for data-only SSE. The
reactive path-form hint patch was retired — subsumed by S8's
description fix. stealth/ox-alpha is certified (max effort, reasoning
streamed, openrouter-responses adapter, config digest 4fd7d48b) and
queued as the next implementer experiment; its usage telemetry will be
the first eval-capture data for an unattributed frontier model.
