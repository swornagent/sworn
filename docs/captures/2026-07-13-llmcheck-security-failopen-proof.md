# Proof bundle — llm-check security fail-open (sworn#103)

Date: 2026-07-13
Anchor: [swornagent/sworn#103](https://github.com/swornagent/sworn/issues/103)
Protocol half: [sawy3r/baton#68](https://github.com/sawy3r/baton/pull/68) (v0.12.0)
Base: `release/v0.1.0` @ `b31de52`

## Scope

Close the fail-open in `sworn llm-check --check security-review`, where a `critical`
finding alongside a model-declared `verdict: "PASS"` passed the gate green.

## Files changed

`git diff --name-only HEAD` plus untracked, from live repo state:

```text
internal/gate/llmcheck.go
internal/gate/llmcheck_blocking_test.go   (new)
```

## Root cause

`HasViolations()` decided whether a check blocked by string-matching
`f.Severity == "FAIL"`. The six checks grade on **two vocabularies**: five use
`FAIL`/`WARN`/`INFO`, and `security-review` uses `critical`/`high`/`medium`/`low` and
**never emits `"FAIL"`**.

So the loop was **dead code for the security check**, and blocking silently degraded to
`r.Verdict != "PASS"` — the model's own self-assessment. Self-certification, in the one
check where a false green matters most.

The two-vocabulary split was not a cosmetic wart sitting next to the bug. It **was** the
bug.

## Test results

`go test ./internal/gate/ -run TestHasViolations -v`:

```text
--- PASS: TestHasViolations_SecurityFailOpen (critical, high)
--- PASS: TestHasViolations_AdvisorySecurityFindingsDoNotBlock (medium, low, info)
--- PASS: TestHasViolations_LegacyVocabularyStillBlocks
--- PASS: TestHasViolations_BlockingEscalatesButCannotDeEscalate
--- PASS: TestHasViolations_UnknownSeverityFailsClosed
--- PASS: TestHasViolations_ModelFailVerdictAlwaysBlocks
--- PASS: TestHasViolations                      (pre-existing, unbroken)
ok   github.com/swornagent/sworn/internal/gate
```

Full suite — `go test ./...`: **47 packages ok, 0 failures**. `go build ./...` clean.

## Guard fidelity (Rule 12)

**Mutation proof.** `HasViolations()` reverted to the original defect
(`if f.Severity == "FAIL"`), suite re-run:

```text
--- FAIL: TestHasViolations_SecurityFailOpen/critical
    FAIL-OPEN: severity "critical" + verdict PASS did not block.
--- FAIL: TestHasViolations_SecurityFailOpen/high
    FAIL-OPEN: severity "high" + verdict PASS did not block.
--- FAIL: TestHasViolations_BlockingEscalatesButCannotDeEscalate
--- FAIL: TestHasViolations_UnknownSeverityFailsClosed
```

Restored, green again. Both halves recorded.

**Mutating the form the defect ACTUALLY takes.** The guard's fixture is the real shape:
`CheckSecurityReview` + `severity: "critical"` + `verdict: "PASS"`. Not an imagined shape —
the exact payload the gate was passing green. A guard written against
`severity: "FAIL"` (the shape one would *imagine* testing) would have passed its own
mutation test and still missed every real instance.

**Scope parity.** The claim is "a blocking finding cannot pass the gate", and the domain is
every check × every severity value in either vocabulary. The tests cover both vocabularies
(FAIL/WARN/INFO and critical/high/medium/low/info), the explicit `blocking` flag in both
directions, unrecognised grades, and the empty-findings case.

**Right instrument.** The blocking decision is now a typed predicate over the finding
(`IsBlocking()`), not a string equality against one vocabulary's magic value.

## Reachability artefact

The affordance is the gate's exit code, which is what every caller acts on
(`cmd/sworn/llmcheck.go:119` → `os.Exit`; also exposed via MCP `internal/mcp/lint.go`).

Before the fix, a reproducing test against the live engine:

```text
FAIL-OPEN: a critical RCE finding did not block the gate.
  check=security-review verdict="PASS" severity="critical"
  HasViolations()=false  -> sworn llm-check exits 0, gate passes GREEN
```

After: the same payload returns `HasViolations() == true`, so the gate exits non-zero.
The protocol half makes it stronger still — under `llm-check-report-v1` that payload is
now **schema-invalid**, verified against a Draft 2020-12 validator:

```text
valid=False want=False  PASS + critical blocking finding (THE FAIL-OPEN)
valid=True  want=True   PASS + advisory only
valid=True  want=True   FAIL + blocking finding
valid=False want=False  FAIL + advisory only (unbacked)
valid=False want=False  old vocab severity 'FAIL' rejected
ALL INVARIANTS HOLD
```

## Delivered

- `LLMFinding.IsBlocking()` — a typed blocking predicate covering both grading
  vocabularies, honouring Baton v0.12.0's explicit `blocking` flag, failing closed on an
  unrecognised grade. Evidence: `internal/gate/llmcheck.go`.
- **Escalate-only asymmetry**: `blocking: true` may promote a `medium` finding to blocking,
  but `blocking: false` may **not** de-escalate a `critical` one — that would reopen the
  fail-open through a different door. Evidence:
  `TestHasViolations_BlockingEscalatesButCannotDeEscalate`.
- The model's `verdict` demoted from sole authority to corroborating evidence: a `PASS`
  can no longer clear a blocking finding. Evidence: every test above asserts against
  `Verdict: "PASS"`.
- Regression guard with recorded mutation proof. Evidence: the Guard fidelity section.

## Not delivered

- **The prompts still ship the old dual vocabulary** in `systemPrompts`
  (`internal/gate/llmcheck.go`). *Why:* the reconciled prompts are published in Baton
  v0.12.0, and sworn pins Baton by semver tag with a digest check, so the engine cannot
  adopt them until the tag exists. This fix is deliberately **defensive** — it reads both
  vocabularies correctly so the fail-open is closed *today*, without waiting on the
  protocol. *Tracking:* sworn#103 + `docs/captures/2026-07-13-baton-engine-prereq-handoff.md`
  (Phase C). *Acknowledgement:* raised with the Coach in the session wrap-up.
- **`LLMCheckReport` is not yet emitted as `llm-check-report-v1`.** Same reason, same
  tracking. Phase C wires the record and its validation.

## Divergence from plan

The task as framed was "reconcile the grading vocabulary" — a tidy-up. Investigating it
found the vocabulary split was itself a live security fail-open in the shipped binary, so
the work split into a protocol change (Baton, gated on the Coach's tag) and an immediate
defensive engine fix (this bundle, not gated on anything). Surfacing the escalation rather
than absorbing it into a "cleanup" commit.
