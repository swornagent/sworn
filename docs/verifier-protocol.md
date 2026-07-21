# Independent verifier protocol and Store lifecycle

Sworn separates model judgment from delivery authority. A verifier may emit one
strict assessment, but only the engine can create a verifier dispatch, stamp a
Baton delivery-verdict envelope, and route committed delivery state.

The v0.3.0 Store lifecycle now carries that boundary from a `reviewable`
submission through a controlled verifier effect and durable verdict admission.
The native Codex verifier worker now closes that internal execution path. Public
`sworn run` composition, repair execution, and integration are not implemented.

## Three closed records

| Record | Owner | Authority |
| --- | --- | --- |
| `control-receipt-v1` / `verifier_dispatch` | engine | Exact submission and candidate requested for isolated review |
| `sworn-verifier-assessment-v1` | verifier model | Decision content only; no delivery authority |
| `delivery-verdict-v1` | engine | Baton envelope bound to the exact assessment and review inputs |

The assessment contains its local schema marker plus only `outcome`, `summary`,
`acceptance_results`, `assurance_results`, and `findings`. It cannot supply a
verdict ID, submission identity, dispatch pointer, agent, review run, freshness
claim, or timestamp. Parsers accept exactly one strict I-JSON object; they do
not scan prose or remove Markdown fences. Verdict construction accepts only
the resulting immutable exact capability.

## Store-owned dispatch

Dispatch starts from active Store ownership and an exact work item in
`reviewable`, or in `retry` with a durably admitted `INCONCLUSIVE` verdict. Store
reloads and rejoins the exact plan and contract, atomic submission identity,
retained builder result and Git candidate, policy-selected checks, evidence,
configured verifier profile, and current state revision. Caller payload is only
the work intent; it cannot choose those facts.

`protocol.BuildVerifierDispatch` derives the submission digest and candidate
commit/tree. The engine stamps Baton's closed isolation claims:

- fresh context is required;
- the builder transcript is absent;
- the target ref is not writable;
- Git remotes are absent; and
- inherited write credentials are absent.

The Store writes the canonical dispatch record as its raw CAS artifact. It then
atomically commits the command, `verifier.dispatched` event, next state, pending
`runner.verifier` effect, and immutable `verifier_dispatch_records` relationship.
The dispatch ID and effect ID are the same Store-derived identity.

Dispatch requires a fresh, short-lived verifier-execution permit. It binds the
controller, run and revision, exact plan and work contract, work attempt,
submission, dispatch and effect, and configured verifier profile. The exact plan
and current source must contain the verifier's `inspect` and `execute` grants.
Store validates the freshly authenticated durable source high-water mark in the
same transaction that commits the dispatch.

Before a pending verifier may actually run, Store derives the request again and
requires another fresh permit at the advanced revision. Generic journal claims
skip verifier effects. A controlled claim records an attempt identity bound to
the effect attempt, dispatch digest, profile, agent, and verification epoch.
Store rejoins current authority immediately before issuing a one-shot
`RunVerifier` capability and retains active controller ownership for the whole
synchronous callback.

Configured startup also checks every pending verifier request before returning
a mutable Store. If its profile digest or agent differs from the process
configuration, startup fails explicitly instead of leaving an effect that the
process can never claim. Ownership activation repeats the check in its locked
SQLite snapshot, closing the race with a prior owner dispatching before it
releases the Store. Completed historical verifier results remain valid under a
later profile rotation because configuration is an execution gate, not a
retroactive validity rule.

The dispatch isolation fields become proven execution facts only after the
native worker rejoins the exact review closure, materializes the retained
candidate afresh, and completes the profile-bound contained invocation below.

## Native memoryless Codex boundary

The process-neutral verifier worker derives one canonical
`sworn-verifier-profile-v1` from the pinned adapter plus the configured executor,
repository, private workspace root, timeout, and materialization ceilings. The
profile is content-addressed and must equal the digest and agent selected by the
Store effect. It binds the exact native binary, explicit model, prompt, output
schema, argv, permission profile, credential mode, executor configuration, and
outer/inner isolation split. Store validates the retained profile bytes when it
later binds or recovers the result; profile rotation does not rewrite history.

The accepted Codex invocation is deliberately narrow:

- `-a never` removes interactive approval prompts while retaining the nested
  sandbox; `--yolo` and every approval/sandbox bypass are forbidden;
- `exec --strict-config --ephemeral --ignore-user-config --ignore-rules` starts
  one new turn with no resume path;
- history and memory use/generation are disabled, project instructions have a
  zero-byte budget, and the working directory is `/tmp`;
- the trusted outer CLI alone has host network and the dedicated CLI-managed
  ChatGPT credential; model-directed tools have neither and use the named
  read-only permission profile; and
- no `--add-dir`, last-message file, API key, model default, inherited process
  environment, builder transcript, Git metadata, remote, or writable target is
  admitted.

The model sees the fresh read-only candidate, canonical plan, submission,
dispatch, and engine-owned assessment schema. It also receives deterministic
`review-*` files: the exact assurance policy and authority receipt plus one
canonical bundle per policy check containing its definition, receipt, local
environment, and base64-encoded stdout and stderr. Those files are reconstructed
from the already durable artifact closure and their names, digests, and sizes
are recorded in the execution receipt.

Codex stdout is treated as a bounded JSONL protocol, not prose. The adapter
requires exactly one thread, one turn, one completed agent message containing a
strict `sworn-verifier-assessment-v1`, and one successful terminal event. The
output schema contains assessment fields only and is sized so one maximum valid
agent-message event remains below the parser ceiling. A failed process, timeout,
cancellation, truncation, malformed event stream, extra final message, invalid
assessment, or cleanup failure returns a control error and manufactures no
assessment or verdict.

After the process returns, the worker mechanically reconciles the exact
content-bound service before claiming quiescence, then removes its private
candidate and input root. A successful execution receipt records the attempt,
systemd unit, profile, candidate workspace digest, complete staged-input
manifest, raw JSONL and stderr CAS captures, fresh thread ID, engine timestamps,
ordinary exit facts, and the absence of an export. It is observation, not
authority: only Store may bind it, and only the later verdict path may interpret
the assessment.

## Typed result and conservative recovery

The verifier effect result is not a verdict. It contains only
`assessment_ready`, the dispatch and verification epoch, CAS pointers to the raw
assessment and execution receipt, and adapter-recorded review times which Store
validates against the journal. Binding it requires the consumed Store capability
and proves:

- exact effect request, dispatch artifact, submission, candidate, profile,
  agent, and epoch equality;
- strict assessment parsing and persistence of its canonical record;
- canonical profile, assessment-schema, and execution-receipt parsing from
  their exact CAS bytes;
- exact plan, policy, authority, check-evidence, executable, staged-input,
  workspace, executor, and isolation closure;
- resolution and size validation of the raw assessment, JSONL stdout, and
  stderr captures, followed by Store-owned replay of the fixed JSONL grammar
  to recover the same assessment bytes and fresh thread identity; and
- a review start no earlier than either dispatch creation or the journal claim.

The result schema also requires `started_at <= completed_at`. Completion and
bound-result recovery refuse to mark the effect succeeded unless the full
review interval fits inside the exact journal lease; verdict preparation repeats
that proof before it can stamp or admit a verdict.

The typed result slot is write-once. Permit expiry after the one-shot worker has
started cannot erase a truthful result. Completion consumes only that already
bound result; generic bind, complete, and fail surfaces reject verifier leases.

If authority is lost or adapter setup fails before `RunVerifier` consumes its
one-shot entry capability, Store may abort that exact claimed attempt. It writes
an attempt-bound `not_applied` observation and returns the same effect to
`pending` at the same verification epoch. Abort and worker entry race on one
shared atomic capability, so both cannot win; a later claim increments only the
effect attempt and does not spend another model turn or review epoch.

Startup recovery is deliberately asymmetric:

- an interrupted verifier attempt with a fully bound, valid result may close to
  `succeeded` through ordinary Store-owned bound-result recovery; and
- an interrupted verifier attempt without a bound result remains `unknown`.
  There is no verifier `unknown -> pending` transition and no text, orphan
  artifact, or operator assertion can manufacture one.

An unbound attempt is an ambiguous possibly-spent model turn, so controller
activation stops. It does not become `FAIL`, `SPEC_BLOCK`, or `INCONCLUSIVE`, and
it creates no verdict. The factual work row remains reviewable while startup
recovery fails closed on the durable `unknown` effect.

Controlled dispatch, verdict admission, and PASS-attention commands can be
converged after restart from stable caller intent: controller, command, run,
work, submission, and verification epoch. Store derives every profile,
dispatch, assessment, verdict, and Baton digest from the immutable journal.
Replay returns an already committed command result without granting execution
authority or repeating a model turn; an absent or differently occupied command
ID remains absent or fails closed.

## Bounded fresh epochs

Verification epochs are durable, monotonically increasing, and capped at three
per unchanged submission. A pending, running, or unknown current effect blocks
redispatch. A succeeded result must be admitted before another dispatch. The
only normal redispatch is from `retry`, where Store proves that the preceding
succeeded effect has its exact admitted `INCONCLUSIVE` verdict. An
`INCONCLUSIVE` result at the third epoch retains that verdict but routes the work
to `attention/replan` instead of advertising an impossible fourth attempt. Each
epoch gets a new dispatch, effect, attempt identity, and write-once verdict;
timestamps do not select current truth.

## Verdict construction and admission

`protocol.BuildDeliveryVerdict` copies only model-owned assessment fields. The
engine supplies the verdict identity, configured agent, review times, and exact
dispatch artifact pointer. Pure binding validation covers:

- exact plan, work contract, target, policy locator/digest, and assurance
  selection;
- submission ID/digest and candidate equality;
- dispatch role, isolation constants, submission, candidate, run, and time;
- raw dispatch artifact digest and exact pointer equality;
- a verifier run distinct from the builder run;
- exact acceptance-result and assurance-pack sets;
- evidence existence, reverse links, and required evidence boundary;
- finding references; and
- declared passing check outcomes and evidence references for `PASS`.

Store preparation then reconstructs the exact succeeded effect, assessment,
dispatch, submission, checks, evidence, candidate, and timestamps from durable
truth and stamps a deterministic verdict record. The caller cannot submit a
verdict projection.

`FAIL`, `SPEC_BLOCK`, and `INCONCLUSIVE` are truthful historical results and may
be banked after authority is later lost. `PASS` has a separate fresh admission
permit bound to the exact assessment and dispatch. Immediately before commit,
Store rechecks the authenticated source high-water mark, exact plan grant
ceiling, retained candidate, policy/check closure, authority receipt, and every
check and evidence artifact. The PASS gate requires no invented workspace grant
beyond the exact plan.

If current authority or control state prevents an otherwise exact `PASS`, Store
does not admit it. A separate controlled transaction preserves the reviewable
row, creates no verdict or effect, and raises delivery-level attention. A later
successful admission supersedes that latch.

Admission atomically writes the canonical verdict, immutable `verdict_records`
relationship, `verdict.admitted` event, and reducer state. One dispatch can have
only one verdict. Current verdict identity comes from committed event order and
the bound verification epoch, not model timestamps.

| Outcome | Work state | Next action | Additional truth |
| --- | --- | --- | --- |
| `PASS` | `verified` | `replan` | Integration is still unavailable |
| `FAIL` | `repair` | `repair` | Exact implementation failure is retained |
| `SPEC_BLOCK` | `blocked` | `replan` | Work and aggregate board expose attention |
| `INCONCLUSIVE` | `retry` | `retry_verification` | A fresh bounded epoch may be dispatched while capacity remains |
| third `INCONCLUSIVE` | `attention` | `replan` | The exact verdict is retained; no fourth turn is possible |

The read-only board exposes the durable verdict ID, digest, outcome, routed
state, action, and attention without querying a second workflow system.

## Digest and persistence boundary

Baton record digests cover canonical JSON; artifact pointers cover exact raw
bytes. The published indented dispatch fixture therefore has a distinct raw
artifact digest and canonical record digest. Store-generated dispatches write
the canonical bytes directly as the raw artifact, so schema v9 deliberately
requires the two persisted digests to be equal.

Schema v9 adds only immutable relationship indexes over the existing journal:
`verifier_dispatch_records` closes dispatch to submission, command, effect,
record, and artifact; `verdict_records` closes verdict to submission, dispatch,
effect, assessment, command, and event. Foreign keys, uniqueness constraints,
and triggers reject mutation, collision, or incomplete provenance. Neither table
is a second scheduler or mutable "current verdict" table. Schema v9 also permits
the narrow same-process `running -> pending` verifier abort described above,
only when the exact claimed attempt has a matching `not_applied` witness and no
result or receipt; it still permits no verifier `unknown -> pending` transition.

A parsed assessment or constructed verdict grants no integration capability by
itself. Integration remains a separate future current-authority and Git
compare-and-swap edge.
