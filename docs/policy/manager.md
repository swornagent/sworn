# Manager decision policy

The policy the Manager seat (ADR-0014) loads on every tick. Each entry names a
situation the engine reports as typed facts, the decision the Manager may take
without the Principal, the evidence it must cite, and the bound it must not
cross. Anything not listed is a Type-1 decision: escalate to the Principal
with evidence identifiers and stop.

Entries are proposed from the decision journal and ratified by the Principal.
Version: 2 (2026-09-22).

## Authority the Manager holds

Delegated only. The Manager may retry, cancel, pause, resume, answer an
attention within an entry below, re-issue a start, and revise a plan when an
entry permits it. The Manager never approves a plan revision, never merges to
main, never changes a roster, and never moves a target ref.

## Entries

### M1. Re-approval of a resealed proposal with unchanged substance

Situation: `authority_state` is `awaiting_approval` for a revision whose bytes
differ from the last approved draft.
Decision: treat as already approved and admit it, when every check holds:
same changed-slice set; unchanged slices byte-identical; changed contracts
differ only in the fields the approval named; the Principal's answer carried
verbatim; `previous_plan`, `approval_ref` and refs as approved; no drift in
scope, checks, host_checks, waivers or dependencies.
Evidence: both plan digests, the diff of the changed contracts.
Origin: catalogue entry 1 (2026-09-05, R7 rev 3).

### M2. Anchor or scope amendment when the work belongs elsewhere

Situation: an implementer declares a substitute file for an `Anchor:` clause,
or a scope amendment is proposed with the Lead's ratification.
Decision: let the run roll forward; no human round trip.
Bounds: minimal file set; no new or retired slice; no changed outcome; no
weakened acceptance; no changed checks or host_checks; never the records root,
contracts/, CI or AGENTS.md. The Verifier still judges sufficiency.
Origin: catalogue entry 2 (ratified 2026-09-12, sworn#309).

### M3. A REVISE whose receipt carries no reasoning

Situation: a Lead receipt whose detail is a probe or pointer body (the engine
now refuses these at submit; a historical one may still exist), or an
attention from the implementer asking whether to resubmit unchanged.
Decision: answer the attention: treat the REVISE as void, resubmit the design
unchanged except for independently judged improvements, restate the decisions
for a fresh review. Cite the receipt identifier and the refusal that preceded
it.
Origin: 2026-09-12 attention on phased-evidence S2.

### M4. A provider refused the dispatch

Situation: the pinned work's failure code is `PROVIDER_LIMITED` or
`PROVIDER_UNAVAILABLE`, with the provider's message in the detail.
Decision: do not retry into the wall. Wait until the message's reset time,
or 30 minutes when it names none, or until a live probe of the lane passes
if that is sooner (probe every few minutes with
`sworn driver probe --config ABS --profile P --model M`; `doctor` makes no
live call and proves nothing here), then retry the work on a fresh epoch.
Cite the failure detail and the probe result. A second park of the same
shape on the same work after a passing probe is Type-1.
Origin: sworn#310 (2026-09-12); refined 2026-09-22 from run
2026-09-22-worker-observability-r2 (three provider admission stalls in one
hour; a retry three minutes after a passing probe recovered the run);
refined 2026-09-23 from S4-lane-live-probe (`sworn driver probe` replaces
`certify` as the cheap, bounded admission probe the seat runs by hand;
`certify` remains the release-wide, separately authorized live check).

### M5. An identical candidate tree resubmitted after a plain host-check fail

Situation: the engine re-executed the check once (sworn#296) and it failed
again, or the recorded failure carries a race or build signature.
Decision: the failure is real. Do not retry the same try. If the contract is
adequate (the Verifier said so), leave the work parked and escalate with the
check output excerpt; if the contract is the problem, propose a revision for
the Principal.

### M6. Try budget exhausted with no defect in the candidate

Situation: exhaustion park whose failed tries are engine defects already
filed (stale_authority, replayed evidence, transport) rather than candidate
defects.
Decision: retry on a fresh epoch once, citing the issue numbers. A second
exhaustion of the same shape is Type-1.

### M7. A run that needs a start unit re-issued

Situation: the serve unit is active, the run reads `running` but no dispatch
effect is claimed and no start command is outstanding (the persistent start
exited when the run parked).
Decision: re-issue the start call. Cite the last event offset.

### M9. A byte-identical plan revision that clears a BLOCKED assembly

Situation: the assembly verification returned BLOCKED for a reason the engine
has since fixed (the record names an engine gap, not a candidate or contract
defect), the run is parked `bootstrap_authority` with outcome `blocked`, and
every slice pass is verified.
Decision: prepare a plan revision whose slice contracts are byte-identical to
the approved revision (manifest metadata only: revision, previous_plan,
approval_ref, and prose naming the record and the fix), pin and lint it, and
put its digest to the Principal. The Principal's approval is still required;
this entry names the route so the seat does not improvise one. Never edit or
remove records.
Evidence: the BLOCKED record identifier, the engine fix (issue and merge
commit), the unchanged slice digests.
Origin: 2026-09-22 (sworn#343, run r3 to r5). Candidate for catalogue entry
3 once a second occurrence confirms the shape.

### M8. Cancel a run at a safe boundary

Situation: the Principal has instructed a stop, or the run is in a loop the
policy cannot break (M5 twice, M6 twice).
Decision: cancel, back up the track ref and any candidate refs, archive any
fenced tree, record the reason.

## Type-1 (always the Principal)

Plan approval and any revision that changes a slice contract; roster or model
changes; moving a target ref; merging a release to main; deleting refs that
are not backed up; anything the engine reports as `human_authority`; anything
this file does not name.

## How an entry is added

A decision the Manager wanted to take and could not, or a Principal answer
that carried no information the engine could not have checked, is written up
from the decision journal with its mechanical predicate and proposed as an
entry. The Principal ratifies it here.
