---
title: 'S38-verifier-blocked-violations — a BLOCKED verdict must populate status.json violations'
description: 'On 2026-06-21 the verifier emitted BLOCKED for S24 and S06a with the reason written only to journal.md prose, leaving status.json verification.violations = []. The loop pages "Verifier BLOCKED with reason: ." (blank) and the Planner has nothing machine-readable to act on. Require BLOCKED verdicts to record the concrete defect/proposed-amendment in status.json violations, and add a deterministic gate that fails a blocked-with-empty-violations verdict.'
---

# Slice: `S38-verifier-blocked-violations`

## User outcome

A verifier `BLOCKED` verdict **always** records its concrete reason (the spec defect /
proposed amendment) in `status.json` `verification.violations`, not only in `journal.md`
prose. The Planner page and `/replan-release` then have an actionable reason, never a
blank `"reason: ."`. A `result: blocked` with empty `violations` is itself rejected as
malformed.

## Why

Observed 2026-06-21: S24-memory-engine and S06a-sworn-login-auth were BLOCKED with the
reason (a `cmd/sworn/main.go` merge conflict + proposed fix) written to `journal.md`, but
`status.json.verification.violations` was left `[]`. The loop's REPLAN page rendered
"Verifier BLOCKED with reason: ." — unactionable. The handoff to the Planner is
non-terminating when the defect isn't in the machine-readable field.

## In scope

- **`internal/prompt/verifier.md`** (and `verify-slice.md` command): on a BLOCKED verdict,
  the verifier MUST write the concrete defect + proposed amendment into
  `status.json.verification.violations` (a non-empty list), in addition to the journal
  prose. State this as a hard requirement in the BLOCKED branch.
- **Deterministic gate:** a check (in `sworn verify`'s result handling, or
  `scripts/release-verify.sh`, or a small `internal/verify` validation) that fails closed
  when `verification.result == "blocked"` and `verification.violations` is empty — so a
  malformed BLOCKED can't be recorded/handed off.

## Out of scope

- Changing how FAIL records violations (already populated).
- The loop's page-rendering format.

## Planned touchpoints

- `internal/prompt/verifier.md` (BLOCKED branch: require violations)
- `internal/verify/` (validation that blocked ⇒ non-empty violations) — verify the package's
  result-recording path first; if recording is prompt-side only, scope the gate to
  `release-verify.sh` and note it.

## Acceptance checks

- [ ] `verifier.md` BLOCKED branch explicitly requires populating `status.json`
  `verification.violations` with the concrete defect + proposed amendment
- [ ] a deterministic check fails closed on `result: blocked` + empty `violations`,
  naming the slice — covered by a unit test
- [ ] a well-formed BLOCKED (non-empty violations) passes the check
- [ ] `go build ./...` + the new test pass

## Required tests

- **Unit**: `TestBlockedRequiresViolations` — a status.json with `result:blocked` and
  `violations:[]` fails the check; with a non-empty violation it passes.
- **Reachability artefact**: run the check against a crafted empty-violations blocked
  status.json (fails) and a populated one (passes); capture in proof.md.

## Risks

- Existing recorded blocked-empty verdicts (S24/S06a) were cleared by the planner already;
  the gate is forward-looking. Ensure it doesn't retroactively wedge a slice mid-flight —
  it gates the *verifier's* write, not historical state.

## Deferrals allowed?

None.
