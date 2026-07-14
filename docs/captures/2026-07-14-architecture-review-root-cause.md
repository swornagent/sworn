# Architecture review: why recorded decisions drifted

Date: 2026-07-14  
Review issue: [#108](https://github.com/swornagent/sworn/issues/108)

## Conclusion

The recurring defect is not a lack of design work. The repository contains many
good decisions, strong fail-closed intent, and extensive tests. The defect is a
missing conversion step between **decision** and **executable invariant**.

Several failures had the same lifecycle:

1. a sound rule was recorded in a spec, ADR, or review;
2. one implementation encoded it locally;
3. a sibling caller re-encoded or bypassed it;
4. tests proved each local component, often through injected fakes;
5. no assembled-path or architecture guard compared the siblings; and
6. the divergence remained green until a real command sequence crossed both
   implementations.

That explains why competent review did not catch the defects. The process was
good at judging surfaced decisions and weak at surfacing implicit ones. It was
good at proving leaf behavior and weak at proving that production selected the
proved leaf. It recorded deferrals but did not govern the lifetime of accepted
compatibility shims. Finally, it treated the issue backlog as storage rather than
as an input to diagnosis and planning.

## How the defects got through

### Recorded decisions were not executable

The provider-key location, canonical environment names, and role-to-model
assignment were written down. Recording established intent, but no test or lint
made the implementation answer to that intent. The review found the same gap in
Sworn's own architecture gate: the rule engine existed, but the project supplied
no populated policy, and malformed policy could result in an empty successful
run.

A design record is evidence for a reviewer, not an enforcement mechanism. If a
decision has a mechanical projection—one credential path, no hardcoded routing
model, adapters do not write authoritative state—then accepting the decision
without that projection leaves the implementation unconstrained.

### Local implementation choices became undeclared architecture

The Captain's model was not deliberately chosen from the implementer retry
ladder. A convenient value crossed a role boundary and became the behavior. A
test then preserved it. The same shape appears in production task execution: a
complete, directly tested engine exists while the CLI selects a second task path
and discards an accepted `--base` value. The architecture was decided by the
caller graph, not by a design decision.

This is why code review of the local change was insufficient. The missing
question was not “does this value work?” but “which component owns this choice,
and can any sibling answer the same question?”

### Compatibility shims had approval but no mortality

The legacy provider environment prefix was reviewed and retained for a concrete
compatibility reason. That satisfied the current no-silent-deferral rule: the
reason was stated and the reviewer acknowledged it. Nothing, however, required
an owner, removal condition, or expiry. A reviewed shim therefore hardened into
an accidental public contract.

Acknowledgement is evidence that a trade-off was understood. It is not a
retirement mechanism. A temporary exception needs a lifecycle as well as a
rationale.

### Tests proved components, not production selection

The suite contains substantial unit and integration coverage. Yet several tests
replace the boundary most likely to drift: subprocess command construction,
router creation, environment/config lookup, or assembled CLI dispatch. A fake
can prove the consumer reacts correctly to a success value while hiding that the
real producer can no longer create that value.

The review reproduced three variants:

- a remote rerun handler reports successful launch while its real CLI shape
  exits with a usage error;
- router construction failure silently selected a retired static path that
  could return PASS; and
- credentials tests covered provider and account writers separately while an
  ordinary sequence of the two writers destroyed fields owned by the other.

There was also an inverse failure: tests read ambient user configuration or
environment and passed for a different reason than the assertion described.
Both cases are scope errors. The guard's claimed domain was larger than its
actual domain.

### State labels were committed separately from their effects

Autonomous orchestration needs terminal labels to be facts, not intentions.
Several paths marked a track or slice done before merge or persistence had
succeeded, discarded terminal write errors, or reduced an empty cancelled run
to success. MCP and TUI could also rewrite the same whole-file status outside the
loop's ownership.

These are not independent missing error checks. They arise because no typed
terminal-outcome use case owns the effect, durable state transition, event, and
notification together. Callers sequence those steps themselves, so order and
failure semantics drift.

### Adapters grew application logic because there was no control core

The CLI loop, MCP server, and TUI each needed useful operations before a shared
control service existed. They consequently acquired their own command strings,
status mutations, authoritative-read choices, and error encoding. The historical
operations harness made the desired operational concepts visible—commands,
events, liveness, restart, parking, and remote actions—but the native
implementation has not yet consolidated them behind one boundary.

Adding a web board directly to today's file operations would create another
copy. The UI should instead force the missing seam into existence: one durable
command/event core, with CLI, TUI, MCP, web, and notifications as adapters.

### The backlog was durable but not reachable

Issues preserve work well, but preservation alone did not prevent duplicate
diagnosis. The review brief itself records a defect that was rediscovered and
fixed before its existing issue was found. Search was a late reporting step, not
an early diagnostic gate.

The process therefore paid twice: once to identify the original problem and
again to rediscover it. Durable capture needs a retrieval trigger at the moment
the repository area and failure signature become known.

## Changes Baton should own

### 1. Decision-to-guard classification

Every approved design decision should state one of:

- **mechanically enforced** — name the test, lint rule, schema, or architecture
  rule and its mutation proof;
- **observationally verified** — name the live or assembled-path evidence and
  its refresh trigger; or
- **not enforceable** — explain why and name the human review trigger.

This extends Rule 12 from defects to decisions. It does not require every
sentence in a design to become a lint rule; it requires an explicit decision
about how drift will be detected.

### 2. Deferral lifecycle, not acknowledgement alone

Rule 2 should require a fourth element for compatibility shims and temporary
exceptions: **retirement**. Retirement is an owner plus at least one of an expiry
date, target release, removal issue, or observable condition that triggers
reconsideration. If no retirement is intended, the record must call the behavior
permanent so reviewers judge it as contract rather than debt.

### 3. Authority declarations for shared concepts

Design review should identify concepts with multiple consumers—credentials,
release-ref resolution, role/model selection, state transitions, command grammar,
and notifications—and name one authority plus its consumer inventory. A new
consumer must either use that authority or record an explicit exception.

This is more actionable than a generic prohibition on duplication. Repeated
syntax is not always architectural duplication; repeated answers to the same
policy question are.

### 4. Guard scope and causal mutation evidence

Rule 12 should require the guard's claim to name its entrypoint and domain. Its
mutation proof must fail for the intended cause, not merely produce the same exit
code through an unrelated error. For user-facing behavior, at least one proof
must drive the production assembly point with real command construction and clean
configuration isolation.

### 5. Backlog lookup as an intake gate

Planning and defect diagnosis should capture repository area, symbols, and error
text, then query open work before implementation. The proof should record
`new`, `duplicates #N`, or `extends #N`. Search failure is not a blocker to an
urgent containment fix, but it must be visible rather than silently skipped.

## Changes Sworn should own

### 1. Enforce its own architecture policy

Sworn should load a populated project policy, reject malformed or empty policy,
and exercise it through `sworn lint design`. The first rules should protect the
confirmed seams: declared touchpoints, one credential authority, no direct state
writes or loop subprocess construction in control adapters, checked terminal
persistence, context-bound orchestration children, canonical provider
environment names, and registry-owned model choices.

Every rule needs a deliberate violating mutation that demonstrates a causal
FAIL. Policy absence may remain optional for adopting repositories, but Sworn's
own test suite must require its policy to exist and remain populated.

### 2. Create one durable control and event service

The loop should be the sole owner of runtime state transitions. Other processes
submit typed commands carrying a target identity, expected revision, and
idempotency key. Reads return the authoritative source reference and revision.
Events are appended durably and describe accepted, started, completed, failed,
blocked, paused, and recovered operations.

This service is the shared foundation for autonomous recovery, MCP parity, the
TUI, a mobile web board, and actionable notifications. It must exist before
remote mutation is exposed.

### 3. Make terminal outcomes one typed boundary

One use case should own outcome validation, status transition, atomic durable
write, canonical Git persistence, event append, and notification enqueue. PASS,
FAIL, BLOCKED, cancellation, pause, and merge failure all traverse the same
boundary. A persistence failure must prevent a success label and must be
recoverable on restart.

### 4. Prove one real terminal journey

The release should contain an offline, deterministic journey that drives the
real binary from a task or release board through planning, implementation,
fresh verification, retry/escalation where applicable, release assembly, and the
chosen final-merge policy. It should prove restart and cancellation boundaries,
not only the happy path.

The product must explicitly choose whether final production merge remains a
human constitutional gate or may proceed under recorded standing delegation.
“Autonomous” should never blur that distinction.

### 5. Build the mobile board as a projection, not an owner

The first web slice should be a responsive, read-only projection of the shared
event/state service. Remote controls follow only after authentication,
authorization, CSRF/origin protection, revision preconditions, idempotency, and
an audit record are in place. The server should bind locally by default and fail
closed if remote exposure is requested without an explicit security
configuration.

### 6. Deliver notifications through a durable outbox

Terminal and attention-required events should enqueue notification records in
the same durable operation boundary. Delivery workers use bounded requests,
retry policy, dead-letter visibility, and replay. Generic webhooks and mobile
push are adapters over the same event and command contracts; inbound actions are
authenticated, authorized, idempotent commands rather than ad hoc status writes.

## The practical test

The protocol and engine changes are successful when a future design decision can
answer four questions without interpretation:

1. Who owns this concept?
2. Which production entrypoints consume it?
3. What executable evidence detects drift?
4. If this is temporary, what makes it end?

If any answer is absent, the review has found a process gap before it becomes a
code defect.
