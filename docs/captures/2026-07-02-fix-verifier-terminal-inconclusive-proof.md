# Proof bundle — fix: verifier terminal dispatch errors surface BLOCKED, not INCONCLUSIVE

Fix ID: `verifier-terminal-inconclusive` (audit finding
`model-provider--verifier-terminal-error-inconclusive`, severity high,
adversarially CONFIRMED). Refs swornagent/sworn#51.

## Scope

`verify.RunAgentic` mapped ALL ChatStructured dispatch errors — including
terminal `model.Error` kinds (KindAuth 401/403, KindCredits 402) — to an
INCONCLUSIVE verdict, so triage walked the full retry/escalation ladder (real
implementer spend) on a revoked key or exhausted credits, and the slice died as
`failed_verification` instead of BLOCKED. This fix checks `model.IsTerminal`
on the dispatch error (the same predicate the implementer path uses at
`internal/run/slice.go` S09 AC1) and surfaces a BLOCKED verdict
(`failed_gate: verifier_terminal_error`), which the existing triage policy
maps to Halt (`internal/orchestrator/triage.go` Blocked → Halt, already
covered by `TestBlockedHaltsCommitsBlocked`). No change was needed in
`internal/orchestrator/triage.go` or `internal/run/slice.go`.

## Files changed

`git diff --name-only 632d4f3` (live):

```
internal/verify/verify.go
internal/verify/verify_agentic_test.go
```

## Test results

`go test -timeout 120s ./internal/verify/... ./internal/orchestrator/...`:

```
ok  	github.com/swornagent/sworn/internal/verify	0.020s
ok  	github.com/swornagent/sworn/internal/orchestrator	0.003s
```

Downstream consumers (RunAgentic result feeds RunSlice triage; CLI):
`go test -timeout 120s ./internal/run/... ./cmd/...`:

```
ok  	github.com/swornagent/sworn/internal/run	4.849s
ok  	github.com/swornagent/sworn/cmd/sworn	39.404s
```

`go vet ./internal/verify/ ./internal/orchestrator/` clean; touched files
gofmt-clean (`internal/verify/concurrent_test.go` was already gofmt-dirty at
base 632d4f3 — pre-existing, deliberately not folded into this fix).

TDD red first: `TestRunAgenticTerminalDispatchErrorBlocked` failed at base
with `expected BLOCKED for terminal auth error, got INCONCLUSIVE`; live CLI
red repro below.

## Reachability artefact

Live end-to-end through the `sworn verify -agentic` affordance (binary built
with `go build -buildvcs=false -o bin/sworn ./cmd/sworn`), non-empty
spec/diff/proof scratch files, bogus OpenAI key (real 401 from
api.openai.com):

RED (base 632d4f3):

```
$ SWORN_DIRECT=1 SWORN_OPENAI_API_KEY=bogus-key-fix1-red bin/sworn verify \
    -agentic -spec spec.txt -diff diff.patch -proof proof.md \
    -verifier-model openai/gpt-4o-mini
{
  "verdict": "INCONCLUSIVE",
  "failed_gate": "verifier_structured_dispatch",
  "rationale": "Incorrect API key provided: bogus-ke******-red. ...",
  "cost_usd": 0
}
EXIT=3
```

GREEN (this fix):

```
$ SWORN_DIRECT=1 SWORN_OPENAI_API_KEY=bogus-key-fix1-green bin/sworn verify \
    -agentic -spec spec.txt -diff diff.patch -proof proof.md \
    -verifier-model openai/gpt-4o-mini
{
  "verdict": "BLOCKED",
  "failed_gate": "verifier_terminal_error",
  "rationale": "KindAuth: Provider rejected credentials — check the API key for openai in ~/.sworn/.env — halting; check verifier provider credentials",
  "cost_usd": 0
}
EXIT=2
```

## Delivered

- Terminal dispatch errors (KindAuth, KindCredits) on the agentic verifier
  path now return BLOCKED / `verifier_terminal_error` / exit 2 — evidence:
  `internal/verify/verify.go` (`model.IsTerminal` check + `blockedTerminal`
  helper), `TestRunAgenticTerminalDispatchErrorBlocked` (both kinds, verdict +
  exit-code paired per repo convention), live CLI artefact above.
- Transient typed errors (KindRateLimit, KindUpstream, KindTransient,
  KindOther) and untyped errors stay INCONCLUSIVE /
  `verifier_structured_dispatch` so triage retries/escalates — evidence:
  `TestRunAgenticTransientTypedErrorInconclusive`,
  `TestRunAgenticStructuredDispatchErrorInconclusive` (pre-existing, still
  green).
- Terminal error → Halt routing: BLOCKED already maps to Halt in triage —
  evidence: pre-existing `TestBlockedHaltsCommitsBlocked` and
  `TestBlockedIgnoresResolveBudget` in `internal/orchestrator`, both green on
  this branch; RunSlice's Halt/Blocked branch commits `blocked` state and
  routes to replan, mirroring the implementer path.

## Not delivered

- No BLOCKED-state notification/status.json integration test through RunSlice
  with a terminal verifier error (would need a full worktree + git fixture
  harness). Why: RunSlice's Blocked→Halt handling is pre-existing, already
  exercised by existing run tests, and this fix only changes which verdict
  RunAgentic emits; the verdict-level boundary is fully covered. Tracking:
  covered by the audit umbrella swornagent/sworn#51. Acknowledgement: recorded
  here and in the fix commit body for the audit orchestrator.

## Divergence from plan

- The finding guidance said "prefer changes in internal/verify +
  internal/orchestrator/triage.go". No triage.go change was needed: triage
  already maps Blocked → Halt (tested), so emitting BLOCKED from RunAgentic is
  the whole fix. internal/run/slice.go untouched, as directed.
