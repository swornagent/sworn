# First real-model `sworn run` — capture, 2026-08-09

Two production runs of release `2026-08-09-provider-capability` driven by
GLM-5.2 over Ollama cloud (`ollama-glm` profile, chat-completions flavour,
default reasoning effort). Both runs parked without completing S1's
implementer-design turn. This capture preserves the wire evidence for run r2,
whose provider traffic went through a loopback logging proxy.

## Verdict

The worker did the job; the harness killed it. Three interlocking expression
limits — none of them provider failures — ended every try:

1. **`MaxProviderTurns = 32`** (`internal/driver/provider.go:11`). A careful
   implementer-design pass over this repo takes ~30 tool turns. GLM-5.2 spent
   turns 1–31 on a methodical, *correct* investigation (work-context →
   contract → the four touchpoint files → the exact defect sites →
   `test/e2e`), then submitted on turn 32 — the last allowed.
2. **`MaxSubmissionSummaryBytes = 280` / `MaxDetailBytes = 8192`.** The
   submission (`submit-033-args.json`) was correctly enveloped, bound to the
   exact invocation and responsibility, and technically accurate — with a
   927-byte summary and 14,996-byte detail. `ValidateSubmission` rejects it
   (`INVALID_SUMMARY`).
3. The submission-correction path engaged — a `malformed_correction` recovery
   step was durably reserved (it is in the r2 journal) — but the turn budget
   was already exhausted, so the invocation returned `RESOURCE_LIMIT`, the try
   died as `runner_error`, and **the model never saw the correction**.

Try 2 (`034-request.json`) then started with a fresh conversation whose only
difference is the try number in `work-context`: no correction content, no
carried exploration. It repeated the same ~30-turn investigation at full
token cost and was killed the same way. Three tries, then the run parks with
`operational_failure` and no recorded reason.

## Files

- `002-request.json` — try 1 opening turn: the model prompt envelope,
  operation instructions, advertised tools.
- `033-response.json` — try 1 turn 32: the `sworn_submit` tool call
  (16,236-byte arguments) that was rejected.
- `submit-033-args.json` — the submission extracted from that call. Read the
  summary: it is an accurate one-paragraph statement of the S1 design.
- `034-request.json` — try 2 opening turn. Diff against `002-request.json`:
  only the invocation try number and work-context digest change.

Journals: `.sworn/runs/2026-08-09-provider-capability-r1.journal` (no proxy,
3 tries, parked), `.sworn/runs/2026-08-09-provider-capability-r2.journal`
(proxied, stopped after the try-1 evidence was captured).

## Secondary findings

- **Failure diagnostics are digested, not stored.** A failed dispatch
  journals only `ObservationDigest` (`internal/runtime/dispatch.go`, attempt
  persistence); the observation body carrying `Diagnostic.Code` is discarded.
  `sworn status` shows `error_code: operational_failure` for every failure
  mode. Same defect class as the receipt `checks` digests.
- **`driver certify` is flaky against reasoning models**: 1 PASS / 3 FAIL
  over four runs. `certificationInvocation` (`internal/driver/factory.go`)
  sets no `RecoveryStepHook`, so a model that opens with prose gets
  `RECOVERY_STEP_REFUSED` instead of the one bounded nudge the runtime would
  grant. The identical invocation with a hook wired passes in ~15s.
- **Sworn's `Read` tool takes only `path`** — the model called it with
  Claude-style `limit`/`offset` (as strings), got `INVALID_TOOL_ARGUMENT`,
  and adapted via `Bash sed`. Bounded and self-corrected; worth knowing.
- **The `reasoning` field is dropped on transcript replay** (known gap,
  `sworn-provider-observability-gap`): try 1 produced a 74KB reasoning burst
  whose content never reappears in later requests.

## What this run proved

The plan-side machinery worked end to end on the first attempt: manifest-v1
plan recorded via `baton.RecordPlanRevision` with `ContractTree`, operator
approval installed, bootstrap authority admitted, track base prepared, real
provider dispatch with the runtime recovery hook, bounded retries, graceful
park, crash-safe journals. Every failure above is an expression limit or an
observability hole, not a trust-machinery defect — and every one of them was
already named in the memory corpus before this run was attempted.
