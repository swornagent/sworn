# Baton Protocol 1.0

Baton specifies responsibility boundaries and the durable facts transferred
between them. It does not prescribe a scheduler, worktree strategy, provider,
model, project-management method, or recovery state machine.

## 1. Responsibilities

Roles are authority boundaries, not personas.

### Planner

The Planner proposes a bounded plan or forward-only revision. It defines the
goal, target, authority, tracks, stable slices, scope, acceptance, checks,
dependencies, consumed inputs, constraints, and exclusions. It cannot approve,
implement, or certify its own plan.

### Implementer

For each attempt, the Implementer first returns a concise design TL;DR and
stops. After an applicable Captain `PROCEED`, it builds the candidate, runs the
required checks, and returns acceptance-linked evidence. It does not review its
own design or issue a delivery verdict.

### Captain

A distinct Captain reviews the exact applicable plan revision and design
attempt. It returns:

- `PROCEED` — implementation may begin;
- `REVISE` — the same slice needs another design attempt; or
- `ESCALATE` — an external decision or revised approved plan is required.

### Verifier

A fresh, read-only Verifier checks the exact candidate against the applicable
approved plan, Captain decision, checks, and evidence. It returns:

- `PASS` — the candidate satisfies the approved contract;
- `FAIL` — the contract is adequate but candidate or evidence is wrong; or
- `BLOCKED` — a trust-critical decision, scope, contract, or authority fact
  cannot be established.

A transport, runner, tool, or persistence failure produces no verdict. The
unchanged candidate may be retried.

### Merge

Merge has no discretionary verdict. It proves eligibility, composes passed
track candidates, obtains fresh verification of the assembled product, and
integrates only the exact passed candidate against the expected target.

The external authorizer remains outside these responsibilities and owns plan
approval, consequential product judgement, and standing authority.

## 2. Plan revisions and attempts

One release has one goal, target, authority, and evolving plan path. Each plan
revision is immutable in Git and approval binds its exact bytes.

Slice identities remain stable:

- `REVISE` appends a design attempt;
- `FAIL` appends an implementation attempt;
- a plan revision retains a slice whose contract and consumed inputs are
  unchanged;
- a changed slice invalidates only itself and the dependency closure whose
  consumed inputs changed;
- a new outcome adds a slice and a removed outcome retires it explicitly; and
- a new release identity is required only when the goal, target, or authority
  is replaced.

Attempts never erase prior candidates or decisions. The applicable attempt is
the latest one whose bindings agree with the current approved plan and inputs.

## 3. Compact receipts

Each responsibility boundary produces one small machine-written receipt. Every
receipt identifies its version, release, optional slice and attempt, role,
result, exact immutable object binding, and concise summary. Role-specific
bindings cover approval, design, candidate, checks, evidence, verification,
expected target, or observed Merge as needed.

The role returns decisions and evidence; it does not construct protocol
records. A receipt writer validates and persists the result. Receipts may use
Git trailers or compact repository records. Baton standardises their meaning;
the reference kit standardises one deterministic representation.

Longer design or evidence documents are optional. When used, a receipt binds
their exact immutable identity. They do not become universal handoffs.

Runtime facts such as workers, leases, retries, tokens, cost, and logs are
engine data, not Baton receipts.

## 4. Binding rules

- Approval binds the exact plan revision and is protected from delivery actors.
- Captain differs from the design producer and binds the plan revision, slice,
  and design attempt.
- Candidate evidence binds the plan revision, slice, attempt, repository,
  candidate, product tree, checks, and relevant Captain decision.
- Verifier differs from the Implementer and Captain, is fresh and read-only,
  and binds the exact candidate and evidence.
- Work `PASS` covers one slice candidate. Assembly `PASS` separately covers the
  exact composed track candidates and complete product.
- Merge binds the applicable `PASS`, exact candidate, expected target, observed
  target, and resulting integration.

A receipt, runtime event, board row, or self-declared boolean alone cannot prove
protected approval, clean verification, Git identity, or effect success.

## 5. Tracks and composition

Independent tracks may advance concurrently. Ordered slices remain serial
inside a track, and only one writer may mutate a track at a time. Dependencies
and consumed inputs come from the approved plan.

A passed track candidate may be composed only through the approved topology.
Composition preserves exact candidate identity and ancestry. After every
required track is present, a fresh Verifier checks the assembled product. Only
that assembly `PASS` permits final Merge.

Worktree names, branch conventions, locks, compare-and-set mechanics,
transaction receipts, and effect recovery belong to the reference kit or
engine. They must preserve the Baton bindings but are not additional protocol
stages.

## 6. Trust stops and operational recovery

Baton blocks only when a trust-critical fact cannot be established: applicable
approval, unambiguous scope or authority, an applicable Captain decision, an
exact candidate and evidence, fresh verification, unchanged verified
candidate, or safe exact composition.

Missing derived status, stale board output, duplicate dispatch, interrupted
execution, a skipped procedural cursor, or a reconcilable Git effect is
operational. An engine reconstructs, retries, or reports it without creating a
plan revision or Baton verdict.

When competing evidence or an external effect cannot be reconciled safely, the
honest result is an operational stop until the trust fact becomes unambiguous.
It is never permission to guess.

## 7. Guided and autonomous use

A guided host may rely on a person to preserve responsibility separation and
record receipts. An autonomous engine additionally proves protected approval,
process and credential isolation, one active writer per track, durable dispatch
identity, resource bounds, effect recovery, and expected-target updates.

Sworn is the reference autonomous engine. Baton remains usable without Sworn
through its portable operations and reference kit.
