# ADR 0008: Close one bounded builder-to-reviewable production vertical

- Date: 2026-07-21
- Status: accepted

## Context

ADR 0007 proved that one exact Codex CLI could run as a networked trusted
control process while its model-directed tool process had neither the model
credential nor network access. It deliberately added no production adapter,
current check authority, check retry proof, controller path, or mutating command.

The next slice must join those boundaries without recreating v0's orchestration
surface. A builder-only command would expose an unsafe stopping point. A poller,
scheduler, adapter registry, provider layer, or recovery workflow would create a
second owner beside the existing reducer, Store, effect journal, and contained
executor. The smallest useful production boundary is one invocation which takes
one already active work item all the way to the existing atomic `reviewable`
admission edge.

## Decision

### Admit one exact Codex builder profile

Add one production adapter for the ADR 0007 binary and no adapter registry. It
accepts only:

- static-PIE `codex-cli 0.145.0-alpha.18`;
- 304,169,008 bytes with SHA-256
  `16db86b6bf81cc426032fd42216dd97e60f97b149272f1f9963845a0675dae94`;
- the adapter-owned OpenAI provider, literal hardened argv, pinned tool-schema
  digest, one executable input, host network, and nested sandbox;
- one explicit deployment-selected model and finite timeout; and
- one credential under the inner name `CODEX_API_KEY`.

Validation retains and copies the original binary descriptor, hashes the bytes
of a private staged copy, inspects that copy as static PIE, and executes only the
copy for its version probe. Path replacement, short copy, digest or size drift,
an ELF interpreter, a different version, and staging residue fail closed. The
builder profile binds the binary facts, argv, model, environment names, tool
schema, timeout, executor configuration, network, nested sandbox, completion
schema, repository, and work root. The model has no default. There is no `PATH`
discovery, silent upgrade, provider SDK, LangChain, or LangGraph runtime.

The outer Codex process is trusted control-plane code. It receives the model
credential and broad host network. Its nested tool process can edit only the
measured workspace and receives neither network nor credential. Sworn never
supplies authority, signing, or integration credentials to either process.

### Make local-check execution current-authorized and recoverable

Keep `checks.dispatch` and `submission.admit` deterministic historical
transactions. Before each pending `check.local` claim, the sole controller
reloads its exact policy-ordered request and freshly authenticates the configured
authority source. One short-lived opaque permit binds the Authority instance,
controller, run and revision, plan, work attempt and contract, succeeded builder
effect, pending check effect, check and definition, content runtime, and source
head. The exact plan and current source must grant `inspect` and `execute`.

Store rejoins the permit to active ownership, durable source high-water mark,
ordered dispatch, runtime configuration, and exact pending row. Generic claim,
prepare, bind, complete, fail, and recovery APIs reject `check.local`, just as
the corresponding raw surfaces reject native builds. A Store-issued capability
admits one worker entry and retains controller ownership for its synchronous
external work.

The stable effect ID remains the Baton check run ID. Each claim also records a
deterministic executor invocation identity derived from effect ID, attempt, and
runtime digest. A bound result converges through the existing typed-result
closure. For an unbound attempt, a one-shot Store capability permits the worker
to prove the exact systemd unit quiescent and remove its deterministic runtime
and candidate-materialization roots. Store seals the opaque cleanup proof to the
issuance. Migration 008 permits only that witnessed local-check
`unknown -> pending` transition; orphan CAS objects and missing results are not
retry proof.

### Expose one bounded convergence, not a loop

Generalize the existing controller only far enough to bind `BuilderService` and
`CheckService`. Startup remains recovery-only until every interrupted effect has
converged. After activation, `AdvanceToReviewable` derives stable,
domain-separated command IDs for build dispatch, check dispatch, and admission
from the exact run, work, and attempt. It then:

1. dispatches or replays the build under current authority and executes its
   exact pending effect;
2. dispatches the complete plan-derived ordered check batch;
3. freshly authorizes, claims, and executes each pending check serially; and
4. performs the existing effect-free historical admission transaction.

It handles ready, active, checking, and already-reviewable work by converging
the same durable attempt. It never polls, waits for new work, advances a second
item, chooses repair policy, obtains a verifier verdict, accepts `PASS`, or
integrates a target.

The public surface is exactly:

```text
sworn run <run> [<work>] --config <clean-absolute-path> [--json]
```

There is no builder-only command. The Store must already contain an exact
planned and activated delivery. The strict `sworn-run-config-v1` file binds the
existing private database, full repository binding, public authority roots and
bundle directories, executor roots and executable paths, finite limits, content
runtime, distinct builder and check roots, and the Codex binary, model, timeout,
and host credential environment name. It accepts no private key, credential
value, provider selection, helper command, or fallback. The JSON result is a
secret-free monitoring projection; SQLite and the board remain authoritative.

## Accepted evidence and non-evidence

The implementation tests the exact Codex profile and its staged-copy race
boundary, hardened argv and completion schema, current check permits and source
revocation, raw Store bypass rejection, one-shot leases, bound and unbound
builder/check restart paths, stable command replay, real Git candidate capture,
content-bound evidence, deterministic admission, strict configuration, and
bounded result projection.

The process-boundary suite builds the real `sworn` binary, proves that `run`
enters the production configuration root, and reaches the exact Codex pin gate
without executing a rejected binary or disclosing its credential. The ADR 0007
real-Codex suite separately executes the accepted CLI against a scripted local
Responses endpoint. That proof is token-free and changes the provider only
inside the test harness. No live OpenAI model delivery was run for this slice;
the implementation therefore does not claim provider model quality or a live
end-to-end OpenAI success.

## Budget gate

The ADR 0007 merged base was 14,432 nonblank, noncomment and 15,971 physical
production Go lines. This vertical is 17,709 semantic and 19,577 physical lines:
a delta of +3,277 and +3,606 respectively.

The net delta groups as follows:

- CLI and production composition (`cmd`, `app`): +887 / +974;
- exact Codex adapter: +421 / +453;
- bounded control, authority, and engine identity: +854 / +953;
- builder/check effects, executor recovery, and producer join: +338 / +379;
- Store capabilities, validation, recovery, and migration 008: +777 / +847.

This is a third explicit architecture stop. The increase closes one real
production boundary rather than adding a scheduler, framework, provider layer,
or second state machine. The count is evidence and a future contraction target,
not permission to weaken authority, recovery, containment, Git measurement, or
atomic admission. Further growth must close the independent-verdict boundary or
replace existing surface.

## Consequences and deliberate limits

Sworn now has one bounded production path from an existing active work item to
reviewable. Builder and local-check external effects require fresh current
authority, are entered only through one-shot Store capabilities, and can recover
without interpreting absence or operator prose as truth. Stable command IDs and
one SQLite journal provide restart convergence without another workflow engine.

This is not yet an autonomous delivery loop. There is no public `init`, plan
activation, config generator, repository discovery, runtime-digest tool,
external authorizer transport, independent verifier, verdict routing, bounded
repair policy, scheduler, `PASS`, or target integration. The 18 Baton
real-boundary cases remain open.

Deployment currently must acquire the exact 304,169,008-byte Codex alpha binary
outside Sworn; there is no installer or update path. A production `sworn run`
uses the built-in OpenAI provider and may consume billable model tokens. Runtime
execution is Linux-only and depends on the systemd user manager, delegated
cgroup v2, Bubblewrap, unprivileged user namespaces, and a finite executable
tmpfs. The host account and same-UID processes remain trusted. `reviewable`
means exact local evidence was admitted; it is not an independent verdict or
`PASS`.
