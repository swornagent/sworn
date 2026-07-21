# Baton Protocol 1.0

This document defines the minimum delivery loop implementing the five principles
in [CORE.md](CORE.md). Baton records facts and authority. The engine owns
mechanics; capable models choose how to do the work.

## 1. Actors and separation

- The **authorizer** approves the delivery plan and any later authority change.
- The **builder** creates a candidate within one work contract and reports
  completion or insufficient authority. It cannot issue a delivery verdict.
- The **verifier** reviews one immutable submission from fresh context. It must
  not change tracked candidate bytes.
- The **engine** validates records, derives state, runs deterministic policy,
  isolates actors, captures repository facts, and performs only authorized
  effects.

These are authority boundaries, not mandatory personas or prompt bodies. One
model product may serve both builder and verifier only through distinct runs
with fresh context and different run identifiers.

An evidence producer is an engine-managed check, observer, or attestation intake,
not another universal model role.

## 2. Records

Baton 1.0 has four portable delivery-record shapes. `control-receipt-v1`
standardizes authority-approval, verifier-dispatch, and integration facts.
Producer receipts remain content-addressed artifacts. None is an additional
delivery record.

### Delivery plan

`delivery-plan-v1` is the contract offered for approval. It contains the
delivery outcome, target, bounded authority grants, an assurance-policy locator
and digest, and a dependency graph of work units. Each work unit contains its
own outcome, path scope, acceptance criteria, and assurance selection.

The plan is immutable for an attempt. An authorized revision creates a new plan
digest. Engines MAY store plans in Git, a content-addressed store, or another
durable system, but dispatches and submissions MUST name the digest they used.

A plan is not active merely because `authority.ref` is present. Before dispatch,
the engine MUST resolve that source and durably issue an authority receipt that
binds the exact plan digest, grants, authorizer identity, and approval time. The
submission links its exact bytes by artifact locator and digest. Missing, stale,
or unresolvable approval fails closed.
The engine MUST authenticate approval against a configured trust root, signature,
or capability unavailable to the autonomous caller, builder, and verifier. The
source and proof bytes MUST be outside their write authority and retained
durably. Caller-supplied JSON, a claimed identity, a TTY, or an operating-system
username is not approval by itself. Baton leaves the proof mechanism to the
engine; it does not enlarge the delivery-record schemas.

Repository-local actions target `workspace`. An integration grant names an exact
repository identity and full `refs/heads/...` target matching the plan. Baton 1.0
does not authorize publishing, deployment, or arbitrary external writes. The
builder's `execute` authority is confined to a workspace sandbox and cannot be
used for an external side effect. The engine MUST bound process count, memory,
CPU, output, run time, and writable temporary storage for every builder,
producer, and verifier subprocess under local policy. A broader source policy
does not enlarge the grants recorded in the approved plan.

Scope entries are normalized, repo-relative path prefixes rather than globs.
Matching is case-sensitive over `/`-separated Git paths. `.` matches the whole
repository; otherwise a prefix matches the identical path or descendants after
`/`. Exclusions win over inclusions. Absolute paths, empty segments, `.` or `..`
segments, trailing `/`, backslashes, and glob metacharacters are invalid.

### Submission

`submission-v1` is constructed by the engine from live state after the builder
finishes. It
binds exactly one work contract to:

- the plan and contract digests;
- the authority receipt;
- the exact base commit, candidate commit, and candidate tree;
- the builder run;
- the active assurance policy;
- actual changed paths;
- check receipts; and
- acceptance-linked evidence.

`changed_paths` is an observed list of literal Git paths, not a scope language;
characters such as `*`, `?`, `[`, `]`, and backslash are data there. The list MAY
be empty when the exact base already satisfies the contract. In that case the
candidate commit may equal the base and integration reconciles as already
observed rather than forcing an empty or unrelated change.

The engine, not the builder's prose, MUST derive candidate identity and actual
changed paths. A candidate MUST be committed and its workspace clean before
submission. The engine MUST materialize the candidate in a fresh workspace and
run required deterministic checks there. It stores content-addressed receipts
before constructing the submission. No non-ignored untracked path or unbound
external input may be required for the build, checks, or behavior. A new
candidate creates a new submission and attempt.

Every evidence producer is an engine-registered run represented in `checks`, not
a builder-stamped claim. A producer may execute a deterministic check, perform a
controlled observation, or admit an external attestation. For live observations
and attestations, the engine records the producer identity, candidate binding,
environment, capture time, and exact artifact just as it does for an executable
check.

### Delivery verdict

`delivery-verdict-v1` binds one submission digest to a fresh verifier run. Its
outcome is exactly one of:

- `PASS` — the exact submission satisfies its contract and assurance policy;
- `FAIL` — the contract is adequate, but the implementation or evidence is
  wrong or incomplete;
- `SPEC_BLOCK` — safe progress requires a changed contract, authority,
  assurance requirement, or product/design decision; or
- `INCONCLUSIVE` — verification could not establish truth because its own
  environment, tooling, or evidence access was insufficient.

The verdict records acceptance results, assurance-pack results, and typed
findings. The engine stamps the immutable envelope and validates the verifier's
structured result. A model emission alone is not a verdict record.
Repeated verification of an unchanged submission creates a new write-once
verdict. The current verdict is the last valid verdict admitted by durable
engine event order; a timestamp or filename does not choose it.

When several problems coexist, the most upstream blocker decides the outcome:
insufficient contract or authority is `SPEC_BLOCK`; otherwise an inability to
establish truth in the verifier's environment is `INCONCLUSIVE`; otherwise a
disproved or incomplete delivery is `FAIL`.
This applies to a verifier finding about the bound contract or grants. A current
resolver failure outside the review is an engine control stop and raises
`attention`; it is never converted into a verdict.

### Delivery board

`delivery-board-v1` is a read-only projection of the plan, immutable records,
engine events, and repository facts. It exists for humans and adapters. Editing
it cannot change delivery state. An integration effect receipt binds the
candidate, expected target revision, authority used, effect time, and
compare-and-swap result. An integrated row binds that exact receipt, submission,
and current verdict; a receipt for an earlier verdict is not transferable.

## 3. The standard loop

For each dependency-ready work unit, a conforming engine performs this loop:

1. Validate the current plan and authority.
2. Prepare an isolated workspace at an exact base revision.
3. Start a builder with the work contract, active packs, workspace, and required
   output schema.
   If it reports insufficient contract or authority, record that control fact
   durably and stop without manufacturing a verdict.
4. Inspect the result and repository. Reject out-of-scope changes or unauthorized
   effects. Capture a clean exact candidate.
5. Materialize the candidate afresh, run policy-required deterministic checks,
   and retain their content-addressed evidence. If a required check fails, bank
   that control fact and start a bounded builder repair attempt without creating
   a submission, dispatching a verifier, or manufacturing a verdict. Otherwise
   construct the submission from engine-observed facts.
6. Start a fresh verifier from an engine-controlled context, with the candidate
   exposed only as read-only review data and with no builder transcript, writable
   target ref, inherited write authority, or activated candidate-local runner
   configuration.
7. Validate and record the verdict.
8. Route only from the typed outcome:
   - `PASS` -> `verified`;
   - `FAIL` -> builder repair as a new attempt;
   - `SPEC_BLOCK` -> stop for plan, assurance, or authority revision;
   - `INCONCLUSIVE` -> retry a fresh verifier over the same submission.
9. If the plan lacks the exact integration grant, remain `verified` and replan.
   If the grant exists but a local or manual latch remains, project
   `ready_to_integrate`; otherwise re-resolve authority and integrate only under
   B5 preconditions.

The happy path therefore requires two model-role invocations: builder and
verifier. Deterministic producers may run as bounded subprocesses, and a coding
agent may make multiple internal model turns.
Planning MAY be another model dispatch, but the plan is not active until it is
schema-valid and authorized.

## 4. Derived lifecycle

An engine MAY use different internal event names, but its board MUST truthfully
project these meanings:

- `waiting` — one or more dependencies lacks a current `PASS`;
- `ready` — the contract is valid and every dependency has a current `PASS`;
- `active` — a builder attempt is in progress;
- `attention` — a pre-submission work-level control stop requires contract or
  authority work;
- `reviewable` — a valid submission exists without a current verdict, including
  after verifier transport failure;
- `repair` — the current submission received `FAIL`;
- `blocked` — a verifier gave the current submission `SPEC_BLOCK`;
- `retry` — the current submission received `INCONCLUSIVE`;
- `verified` — the exact submission has `PASS`, but the plan lacks its exact
  integration grant and must be replanned;
- `ready_to_integrate` — the exact submission has `PASS` and the plan grant,
  with only a local or manual integration latch remaining;
- `integrating` — an authorized integration effect is durably in progress; and
- `integrated` — a valid effect receipt exists and the exact verified candidate
  is equal to or an ancestor of the observed target.

These labels are projections, not mutable commands. No success label may appear
before its underlying effect has completed durably.

A delivery-level `attention` latch MAY coexist with a row's factual state. For
example, authority loss after submission leaves the row `reviewable` while the
delivery stops for attention; clearing the latch requires new durable authority,
not an edited board.

## 5. Invalidation

A verdict ceases to authorize integration when any of these bound facts changes:

- its referenced plan becomes unavailable or revoked;
- its extracted work-contract digest;
- base commit;
- candidate commit or tree;
- active assurance profile, pack set, or policy digest;
- authority receipt, source digest, or grants;
- verifier independence; or
- target revision required for compare-and-swap integration.

An unrelated new plan revision does not invalidate already banked work under an
earlier still-authorized plan. The submission continues to name the exact plan
and authority receipt that govern it.

An engine MUST check invalidation immediately before integration. It MUST NOT
repair record drift by rewriting the candidate after verification.

Invalidation prevents a pending effect; it does not erase a completed one. Once
integration succeeds, later source expiry, revocation, or policy change leaves
the banked effect valid. Historical validation uses the source snapshot and
authorization bound by the effect receipt, while new effects use current policy
and authority. The engine MUST durably retain the exact resolved source bytes so
the receipt's `source_digest` can address and revalidate that snapshot.

Protocol records SHOULD be persisted outside the product candidate tree. If an
engine uses a repository metadata channel, updating that channel must not change
the candidate commit or the target's product bytes.

## 6. Composition

The simplest conforming strategy is serial delivery: activate a work unit from
the current target, verify it, and integrate it by fast-forward compare-and-swap
before the next dependent unit begins. The base and candidate MUST belong to the
plan's repository, the base commit MUST be an ancestor of the candidate, and
`changed_paths` MUST equal the actual base-tree-to-candidate-tree diff. Rename
scope checks cover both the old and new path. No dependent builder may start
before every named dependency has `PASS`; its materialized base must include the
exact dependency candidates under B5.

Parallel building is optional. When the target moves, another candidate cannot
be rebased, merged, or cherry-picked under its old verdict. The engine must
capture the resulting candidate as a new submission and verify it. An engine
that cannot prove safe composition MUST serialize the work.

After later serial work advances the target, an earlier integrated row remains
integrated only while Git proves that row's exact candidate is still reachable
as an ancestor of the target. Equality is required at the compare-and-swap
instant; reachability preserves the historical result afterward.

## 7. Retries and stopping

Retry budgets are engine policy, not model judgment. Transport failures never
become `PASS` or `SPEC_BLOCK` by inference. Repeating the same failure SHOULD
stop at a bounded limit and surface the durable findings.

The engine MUST stop when authority is insufficient, repository truth is
ambiguous, persistence fails, the candidate changes during verification, or a
required assurance pack cannot run. Stopping safely is a successful control
outcome, not a delivery verdict.
