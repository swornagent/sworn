# Baton Protocol 1.0

This document defines the smallest complete workflow implementing
[Baton Core](CORE.md). Baton specifies responsibility boundaries and durable
handoffs. It does not select a provider, model, agent host, scheduler, or
project-management method.

## 1. Responsibilities

Roles are authority boundaries, not personas. A human, conversational agent,
subagent, CLI agent, or autonomous engine may perform one when it honours the
same contract.

### Planner

The Planner turns intent into a bounded release plan. It defines outcomes,
scope, acceptance criteria, checks, constraints, ordered work, tracks,
dependencies, touch surfaces, repository, and target. It may replan blocked
work. It does not approve its own plan, implement it, or certify delivery.

### Implementer

The Implementer first writes a concise design and stops. After a current Captain
decision permits it to proceed, the Implementer builds one candidate and writes
acceptance-linked proof. It does not review its own design or issue a delivery
verdict.

The same Implementer context may resume after Captain review. Its candidate
MUST remain inside the approved work and its owning track.

### Captain

The Captain is a distinct invocation that reviews the proposed design during
implementation. It binds the exact plan and design and returns one outcome:

- `PROCEED` — the design is suitable for implementation;
- `REVISE` — the Implementer must revise the design; or
- `ESCALATE` — an external decision or newly approved plan is required.

The Captain does not become another Planner, Implementer, or Verifier.

### Verifier

For work, the Verifier receives the approved plan, current Captain-reviewed
design, exact candidate, and Implementer proof. For assembly, it receives the
approved plan, exact assembled candidate and component heads, and
Merge-prepared proof; assembly has no Implementer design or Captain binding.
Either review runs in a clean context, cannot alter the candidate, and returns
one outcome:

- `PASS` — the exact candidate satisfies the approved contract;
- `FAIL` — the contract is adequate but implementation or evidence is wrong;
- `BLOCKED` — safe progress requires a changed contract, authority, or external
  product decision.

A transport, runner, tool, or environment failure produces no verdict. A fresh
retry may review the unchanged candidate.

### Merge

Merge proves eligibility and composes or integrates the exact passed candidate.
It has no discretionary model verdict. It either performs the authorized,
expected-target Git operation and records the observed result, or stops.

Merge has three mechanical scopes: compose an eligible frozen track and record
the collective authority transfer; prepare the assembled release proof and
status for a fresh Verifier; and, only after assembly `PASS`, integrate that
exact candidate into the release target.

The external authorizer remains outside these five responsibilities. It owns
approval, consequential product judgement, and any standing authority granted
to autonomous execution.

## 2. Release topology

A release has one assembly lineage and one or more ordered tracks:

```text
target
  <- release-wt/<release>
       <- track/<release>/<track-id>
```

- `release-wt/<release>` owns the approved plan, baseline statuses, composed
  track heads, assembly proof, and release Merge record.
- `track/<release>/<track-id>` owns the ordered work assigned to that track
  after materialisation.
- Work advances one item at a time in a track.
- A work at `merge / ready / merge` has passed for track sequencing, so the next
  ordered work may begin. It does not claim that the track has been composed.
- Independent, dependency-ready tracks may advance concurrently.
- A dependent track starts from a release head that already contains every
  required frozen track head.
- Parallel tracks have disjoint declared touch surfaces. An unexpected
  conflict stops for repair or replan.

Only one writer may advance an owning track at a time. Every durable transition
names the exact ref head it observed. The reference helper creates a
record-only commit and updates the ref with compare-and-set; a stale writer
leaves the ref untouched.

Materialisation is one atomic ref transaction. The release ref and newly
created owner ref first point to the same record-only marker, which records one
exact release base and dependency-head set for every work in the track. The
release lineage must retain that marker. Deleting the owner ref, or resetting
release records to make the materialisation appear not to have happened, fails
closed.

Ownership does not move because another branch has a newer timestamp. Before a
track materialises, its release baseline is authoritative. While it is active,
its owning ref is authoritative and a missing or malformed owner record is an
error. Authority returns to `release-wt` only after Git proves that the exact
frozen track head was composed and the matching Merge binding was recorded.

After materialisation, `ESCALATE`, `BLOCKED`, or assembly `FAIL` cannot rebind
the existing identity. The Planner creates a newly approved work and release
identity; the old lineage remains durable archaeology. `REBOUND` is limited to
a pristine, unmaterialised release baseline whose plan or approval changes.

### Mechanical action surface

An engine exposes one safe mutation surface over an admitted plan:

```text
installApprovedPlan   reboundPristinePlan   recordTransition
materializeTrack      composeTrack          prepareAssembly
integrateRelease
```

Actors author plans, statuses, designs, and proofs. They do not supply arbitrary
refs, paths, Git commands, admission capabilities, commit messages, or merge
targets. The action surface derives those from the admitted plan, validates the
prospective immutable commits before any ref moves, and then applies one exact
ref transaction. Retrying an already completed exact action returns the same
durable result without another commit. A divergent state or stale ref stops.

## 3. Durable handoffs

The standard release root is:

```text
.baton/releases/<release>/
  plan.md
  work/<work-id>/
    design.md
    proof.md
    status.json
  assembly/
    proof.md
    status.json
```

Baton 1 uses exactly `.baton/releases`. Plan metadata records that fixed value;
it is not a configuration seam. Any other, absolute, escaping, ambiguous, or
symlinked root fails closed.

Plan, design, and proof identities are
`sha256:<64 lowercase hexadecimal characters>` over their exact raw bytes.

### Plan

`plan.md` starts at byte zero with one strict JSON metadata block:

````text
```baton-plan-v1
{"schema_version":"baton.plan/v1", "...":"..."}
```

# Human-readable release plan
````

No content may precede the opening fence. The metadata has a closed shape and
defines:

- release, repository, target and release-worktree refs;
- canonical record root and protected external approval reference;
- ordered tracks, exact track refs, dependencies, and touch surfaces; and
- each ordered work item's outcome, path scope, acceptance criteria, checks,
  constraints, and dependencies.

The complete file's raw digest is the plan identity. Approval evidence binds
that digest; approval never edits the plan it approves.

### Design

`design.md` is the Implementer's concise proposed approach. Its raw digest and
producer invocation are recorded before Captain review. A revision has new
bytes and therefore a new digest. A Captain decision over an earlier digest
cannot authorize the revision.

### Proof

Work `proof.md` names the delivered outcome and links each acceptance criterion
to observable evidence. Its status binding records the exact repository, base,
candidate commit, normal Git tree, product-tree digest, required checks, and
Implementer invocation.

Assembly proof is prepared by Merge after every exact track transfer. It names
every composed track and frozen head and binds the pre-preparation release head
as both its base and candidate because assembly verifies the complete product,
not a work delta. Per-work verification is not authority to ship an unverified
composition.

### Status

`status.json`, validated by `work-status-v1`, is the sole machine-authoritative
current projection. Every status binds the plan, approval, current
responsibility, proof, Verifier result, and Merge result when present. Work also
binds its design and Captain decision; assembly instead binds its exact
components in the Merge-prepared proof.

Its durable vocabulary is exactly:

```text
stage:      plan | design | implement | verify | merge
status:     ready | blocked | complete
next_role:  planner | implementer | captain | verifier | merge | none
outcome:    none | proceed | revise | escalate | pass | fail | blocked | merged
```

`active` may be a runtime board overlay. A runner failure is `NO_VERDICT`: the
durable status remains byte-for-byte unchanged and the same candidate may be
redispatched. Persisting `active`, `no_verdict`, or any replacement result is
invalid. `NO_VERDICT` is the only unchanged redispatch path.

Git provides history and timestamps. Status contains no transcript, event
array, activity log, retry ledger, worker, lease, token, or cost state.

## 4. Standard transitions

After external approval, each initial work status is
`design / ready / implementer`.

| Current | Responsibility result | Next |
| --- | --- | --- |
| `design / ready / implementer` | design written | `design / ready / captain` |
| `design / ready / captain` | `PROCEED` | `implement / ready / implementer` |
| `design / ready / captain` | `REVISE` | `design / ready / implementer` |
| `design / ready / captain` | `ESCALATE` | `design / blocked / planner` |
| `implement / ready / implementer` | candidate and proof written | `verify / ready / verifier` |
| `verify / ready / verifier` | `PASS` | `merge / ready / merge` |
| `verify / ready / verifier` | `FAIL` | `implement / ready / implementer` |
| `verify / ready / verifier` | `BLOCKED` | `verify / blocked / planner` |
| `verify / ready / verifier` | runner failure / `NO_VERDICT` | unchanged; redispatch same candidate |
| `merge / ready / merge` | exact composition or integration | `merge / complete / none` |

A work `PASS` means its Captain-reviewed design and Implementer candidate proof
passed independently. It leaves the status at `merge / ready / merge` on the
owning track and admits the next serial work; it does not pass the assembled
release. When every ordered work item is there, the exact final track head is
frozen and composed once. One following record-only commit transfers every work
status to `merge / complete / none` together. Partial transfer is invalid.

A materialised track may perform one projection-preserving authority transfer
from its release baseline to its exact owner ref. A `REBOUND` may change plan
and approval only for a pristine, unmaterialised release baseline. Any replan
after materialisation creates new work and release identity rather than
clearing gates in place. Neither operation invents lifecycle progress.

Assembly uses the same status shape with `kind: assembly`, no work or track
identity, and the release-worktree as owner. After every work transfer is
complete, Merge atomically prepares the proof and initial
`verify / ready / verifier` status, then hands it to a fresh Verifier. Assembly
`PASS` means that exact set of components and the Merge-produced assembly proof
passed together; only this permits release Merge. Assembly `FAIL` persists as
`verify / ready / planner`, while assembly `BLOCKED` persists as
`verify / blocked / planner`. Either requires `baton-plan` to create a newly
approved work and release identity; there is no in-place assembly repair or
`RETRY_ASSEMBLY` transition.

## 5. Binding rules

- Captain invocation differs from the design producer and binds the current
  plan and design digests.
- Implementation requires a current `PROCEED`.
- Work proof binds the current plan, approval, design, Captain invocation,
  repository, base, candidate, candidate tree, and product tree.
- Assembly proof binds the current plan, approval, repository, base, assembled
  candidate, candidate tree, product tree, and exact ordered component heads.
- For work, the Verifier invocation differs from the design producer, proof
  producer, and Captain. For assembly, it differs from the Merge proof
  producer. Its trusted dispatch evidence resolves outside the candidate and
  attests clean context and read-only candidate access.
- Verification binds the current proof, candidate, and product tree.
- Each Work Merge binds its own passed candidate plus the shared frozen track
  head, expected and observed release-worktree head, composition result, and
  authority-transfer commit. Every work candidate is an ancestor of the frozen
  head; the frozen product tree equals the final work's passed product tree.
- Release Merge binds the passed assembly candidate, expected target head, and
  observed integration.

Any stale or mismatched binding fails. A runner result boolean or status field
alone never proves separation, evidence, Git history, or effect success.

Structural parsing and the board establish record shape and current authority
only. Before an admitted transition or Merge action, a trusted external
resolver must verify the exact approval and Verifier-dispatch bytes and their
protected provenance. The engine mints an opaque admission bound to that exact
status and execution profile; a copied status, self-declared boolean, or board
row cannot substitute for it.

## 6. Product identity and composition

Candidate proof records the ordinary Git tree and a deterministic SHA-256 over
the ordered path, mode, type, and object identity outside the fixed Baton record
root. Later record-only commits may preserve that product identity.

The exclusion is valid only while the record root is behaviorally inert. If a
build, test, package, deploy, hook, or runtime consumes it, the exclusion cannot
be claimed and validation stops.

Product-tree equality is necessary for record-only advancement but never
replaces ancestry or expected-target checks. Track composition is either an
exact fast-forward to the eligible frozen track head or a two-parent commit
whose ordered parents and tree equal Git's deterministic merge of the expected
release head and that track head. Release integration applies the same rule to
the expected target and passed assembly candidate.

Candidate admission replays the full first-parent history from the exact
materialisation or prior passed-candidate base. Product commits are admitted
only after the current work has Captain `PROCEED`; record commits must form the
closed lifecycle, may not span work identities, and later work cannot advance
before earlier work `PASS`. The final candidate commit is product-only.

After all tracks are composed, Merge prepares the assembly handoff and a fresh
Verifier checks the complete approved plan over that exact product. Only that
assembly `PASS` permits release Merge.

## 7. Guided and autonomous use

A guided host may rely on a human to approve the plan, start distinct Captain
and Verifier invocations, and preserve read-only verification. It MUST stop
when it cannot establish a required boundary.

An autonomous engine additionally proves protected approval, process and
credential isolation, one active writer per track, durable dispatch identity,
resource bounds, effect recovery, and compare-and-set updates. These are engine
mechanisms and executable conformance cases, not prose every model must load.

Sworn is the reference autonomous engine. Baton remains usable without Sworn
through its portable operations and reference record tools.
