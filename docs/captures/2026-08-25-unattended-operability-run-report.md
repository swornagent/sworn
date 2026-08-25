# 2026-08-23-unattended-operability — run report (2026-08-25)

Release complete: 8/8 slices verified, assembly passed, merged to
release/v1.0.0 @ 59edbb24, post-merge repins @ 3cf5074e + 7bce8f9e,
**CI green at 7bce8f9e (run 32808094503) — the release evidence gate.**
Promotion to main awaits the operator's call.

## What shipped

- **S1 credential-preflight** (#221): stale/rotating credentials refuse
  loudly (CREDENTIAL_STALE) before burning tries; native auth-class
  failures distinguished from transport; benign rotation stops
  discarding completed work.
- **S2 claimed-recovery** (#207): expired ownerless claimed dispatches
  reconcile on re-entry instead of RECOVERY_UNCERTAIN; the board's
  needs-you row derives from the gates the verbs actually check; control
  refusals carry their causes.
- **S3 honest-yield-parking** (#191): first question/blocked yield
  parks answerably (durable, crash-barrier, one per work per cycle)
  before any automation runs; later asks keep the nudge doctrine.
- **S4 economy-guards** (#205 g2+3): manifest per-work turn and
  output-token budgets bind on every path;
  limits.identical_failure_park_after (default 2, cap 3) parks
  repeated identical failures early.
- **S5 continuation-lifetime** (#204): the 24h retention constant
  became limits.max_continuation_lifetime_ms (default 24h, cap 30d);
  mid-yield expiry degrades with the labeled fallback_expired fact
  instead of hard-failing; the four yield-path INVALID_CONTINUATION
  sites carry distinct labels.
- **S6 provider-dialect-tolerance**: the two operator ox patches landed
  as product with tests (OpenRouter usage decorations; data-only SSE
  payload-type fallback) plus provider-reported usage.cost capture in
  USD micro-units and the chat root provider field admitted on
  openrouter_chat only (un-stales qwen-via-openrouter).
- **S7 replan-survivability**: yield-first steering at sworn_submit
  (planner plan bytes leave the driver only from the answer-resume
  shape — a bounded correction, not a burned try); sealed proposal
  ExactBytes persist on a child effect at seal, surviving sandbox
  death (the #195/#210 postscript class that twice forced operator
  log-archaeology reconstruction this release).
- **S8 malformed-toolcall-correction**: provider tool-call corruption
  becomes a bounded in-conversation correction instead of a dead
  dispatch. Evidence base: ox-alpha's dup-key Read batching, 0/6 tries
  across two auditions (r1, r6 journals).

## Run ledger

19 run manifests (r1–r19), 3 plan revisions (rev2 S2 e2e scenarios;
rev3 S3 turn-recovery e2e — operator-carried, contract and plan
reconstructed BYTE-EXACT from the killed planner's logged tool calls:
plan sha256 c25fb5eb and canonical digest fc335dfb both matched the
sealed proposal), 1 captain escalation (S3, a real contract gap), 1
verifier FAIL→fix→PASS cycle (S7), ~7 roster configurations across 6
engines, zero verified work lost anywhere (receipts adopted by
ancestry across every relaunch).

Roster history and why it moved: deepseek-pro implemented S1–S4
(account drained to -$0.02 mid-S5), ox-alpha audition failed 3/3 in
100s on the S8 defect class (second audition; datum banked), gemini
implementer hit its 1M window after 47 min (#236 measured: 93% of
conversation mass was whole-file Read results), sonnet died 5/5 at
~20 min to the capture proxy (#237; operator patch #6 later validated
when sonnet ran the entire S8 slice + assembly + merge), grok
delivered S5–S7 (windowed twice at ~490K, #236 second instance),
opus parked on the known US-peak subscription hangs, luna
(gpt-5.6-luna via codex CLI) implemented S7 in 6 minutes but could
not run checks — the codex-native closure mounts no /usr, so no
toolchain (#238) — and her honest answerable park was R3-S3's first
production outing, working exactly as designed. Sonnet closed the
release.

Provider incidents survived: claude CLI 2.1.234 deprecated server-side
mid-run (#220's trigger fired live; re-pinned to 2.1.241 by operator
patch — the merged tree still pins 2.1.234, #220 remains the proper
fix), deepseek balance exhaustion, two context-window classes, one
sandbox toolchain gap, one capture-proxy incompatibility.

## Post-merge repins (3cf5074e, 7bce8f9e) — zero product defects

Four mechanisms, all test-side, none seen by any per-slice verifier:

1. W8 corpus P02/P04 + broker test: stale pins vs S7's ratified
   yield-first (answer-resume shaping; S7's implementer repinned four
   sibling sites and missed these — the N-places prior inside a repin
   sweep).
2. Native credential-preflight tests: defective from birth — fixtures
   mutated Request.Limits after the permission digest was minted,
   failing PERMISSION_BINDING_MISMATCH before any process launched;
   the S1 verifier environment skipped them silently.
3. TUI PTY A1/A2: one-shot journal snapshot raced git-truth
   "complete"; both sites now poll under the deadline.
4. e2e parked-lane scenario: S4's park-after-2 guard parks before t3
   exhaustion; topology fixtures raise the knob to the cap so the
   scenarios keep testing exhaustion + exact retry.

## Findings ledger (carried forward)

- Verifier blind spots, three fresh instances: never-executed tests
  passing via environment skip; S7/S8 check evidence that cannot have
  come from a clean run of the listed command; assembly verification
  not executing the full suite. Strongest case yet for the verify-node
  execution lens + check-evidence provenance
  (docs/captures/2026-08-25-in-engine-learning-spec.md).
- New issues from this release: #235 (operator-surface tails), #236
  (read economics, measured), #237 (capture proxy vs CLI auxiliary
  requests), #238 (codex closure toolchain gap). Live datum on #220.
- Close-with-evidence now unblocked (post-promotion): #191 #204 #205
  #207 #221; #227 remainder to verify.
- The in-engine-learning spec (preflight admission + capability/outcome
  graph) was drafted directly from this run's operator interventions —
  every roster swap and park diagnosis maps to a fact the engine could
  have consulted at admission.
- ox-alpha re-audition unblocked: S8's correction machinery is merged.
