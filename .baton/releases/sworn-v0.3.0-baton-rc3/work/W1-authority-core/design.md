# W1 authority-core design

Implementer invocation:
`codex:/root/w1-rc3-design-implementer/sworn-v0.3.0-baton-rc3/T1-authority/W1-authority-core/design/1`.

## Authority and fixed inputs

This design is for `sworn-v0.3.0-baton-rc3`, track `T1-authority`, work
`W1-authority-core`, and no other work.

- Design materialization and owner head:
  `c846e8d8b9c1e054657e4b94dd586c4b8e7afac7`, clean, on
  `refs/heads/track/sworn-v0.3.0-baton-rc3/T1-authority`.
- Materialization base:
  `0f8d1dea208620cb34226670e66ad1632c0a3402`.
- Frozen dependency `T0-admission`:
  `7d925851dc91a4ee324d9fe29c33d631f44d1a56`.
- Plan:
  `sha256:66dd2c09b538b4eb41783128b3c4d110d10d04124a15f46b2d354858b2409d74`.
- Protected approval:
  `sha256:35415561708d3421c1272fbebada8e56748166077313ba1b540ec96736bc7602`.
- Pinned public Baton RC3 identity: annotated tag object
  `34324784694696a38d951061c2313363b405c1e4`, peeled commit
  `affaf16cc37f845b5dc43b22988d8b680ff1f212`, tree
  `a26078b7db4ee36bdae4f28a48447ff2df782f4f`, generated support package
  `sha256:e5927a82f7c8a0daf3aa1196e7aa56231044449bb141cc2d7efd1cc8cca209bd`,
  and `reference/records/git.mjs`
  `sha256:e66e4bd771651154d2feee6efb35a852047115bffb99846d605958ddd80826f0`.

At implementation start, recapture the exact post-design, post-Captain owner
head. It must be a first-parent, record-only descendant of `c846e8d8`, retain
the same non-record product identity as the materialization, retain the plan
and approval above, remain `implement/ready/implementer`, and contain a current
Captain `PROCEED` for the exact bytes of this design. The target, release,
dependency and materialization bindings must not have moved. The product
candidate is one product-only child of that captured owner head; Baton actions,
not product implementation, later own proof and status commits.

The old product is non-authoritative implementation input only. Before using
it, reconstruct the scoped delta
`b2bb28778a4f73f5841bb936a993860d0b10d0ce..c8ddcd89cbfdaab6c94d601631f87840bf7fd7c2`
restricted to `internal/baton`, `internal/gitx`, and `tools/batongolden`.
Require exactly 26 paths, raw newline path-list digest
`sha256:378cc9b73fdba1a5d24b1f77787c17c41bd151def6ec73855c819f15fed948d7`,
and binary full-index patch digest
`sha256:0a07ef1d8a9d1ec59ea206c3af8b50004bd8244a052f2902e99db04ba78a6f5e`.
Apply or inspect only those product bytes after `PROCEED`. Never copy or cite an
RC2 plan, approval, status, design, proof, verdict, receipt, or history as
authority. The replay is a starting point and is not a candidate until all RC3
gaps below are closed.

## Exact path closure

The candidate changes exactly these 26 paths and no others:

```text
internal/baton/actions.go
internal/baton/actions_integration_test.go
internal/baton/actions_tracks.go
internal/baton/api_external_test.go
internal/baton/candidate.go
internal/baton/evidence.go
internal/baton/lifecycle.go
internal/baton/plan.go
internal/baton/reader.go
internal/baton/records_test.go
internal/baton/repository.go
internal/baton/status.go
internal/baton/strictjson.go
internal/gitx/doc.go
internal/gitx/prepare.go
internal/gitx/refs.go
internal/gitx/repository.go
internal/gitx/repository_test.go
tools/batongolden/main.go
tools/batongolden/main_test.go
tools/batongolden/oracle.mjs
tools/batongolden/testdata/corpus/actions.json
tools/batongolden/testdata/corpus/git.json
tools/batongolden/testdata/corpus/lifecycle.json
tools/batongolden/testdata/corpus/manifest.json
tools/batongolden/testdata/corpus/records.json
```

`internal/baton/assets.go`, its embedded RC3 snapshot, `go.mod`, `go.sum`,
`cmd/**`, `internal/driver/**`, `internal/journal/**`, `internal/runtime/**`,
and `.baton/releases/**` remain byte-identical in the product candidate.
Tests and development tooling remain outside runtime-production accounting.
No new runtime file is permitted.

## Implementation sequence

1. Verify the post-`PROCEED` start, the old 26-path input, the RC3 tag/commit/
   tree/support/`git.mjs` identities, and a clean worktree. Capture all relevant
   refs and old-lineage refs before editing.
2. Replay the scoped old product input without its control history. Immediately
   replace RC2 comments, identities, oracle source bindings, and incomplete Git
   behavior with the RC3-bound mechanisms below.
3. Implement the one literal Git boundary and one exact Snapshot model in
   `internal/gitx` and the `internal/baton` adapter. Complete the exact-ref
   transaction, bounded process, reconciliation, and cleanup behavior before
   connecting actions.
4. Implement strict plan/status/lifecycle parsing, ownership selection,
   evidence binding, product identity, record preparation, deterministic
   composition, and the admitted action facade. Every mutating operation must
   derive refs, paths, parents, messages and merge mode from the admitted Plan
   and current Snapshot.
5. Regenerate the RC3 development corpus twice independently, compare complete
   bytes, then check in the agreed corpus. Port and pass all eleven RC3
   exact-ref adversarial cases in Go.
6. Run focused, race, formatting, scope, identity, candidate and budget gates.
   Simplify within W1 runtime surfaces until the exact line/file budget is met;
   never compensate with tests, tooling, generated files, fixtures or W2.
7. Commit only the 26 product-scope paths as the candidate. Recheck refs,
   candidate ancestry/tree/product identity, path closure, checks, cleanup
   evidence and the planned W2 composition before rendering proof.

## One Snapshot and ownership invariant

There is one consumer-visible `internal/baton.Snapshot`. It is the only exact
ref observation accepted by authority selection or action preparation. Its
fields remain private and immutable. It binds:

- the canonical repository root and admitted absolute Git executable;
- repository object format (`sha1` or `sha256`);
- the raw-byte-sorted unique requested full `refs/heads/**` vector;
- for each ref, one typed state: `direct-commit` with full format-correct OID,
  `absent`, `symbolic` with an exact target when readable, `broken`,
  `non-commit`, or `unreadable`;
- a canonical versioned SHA-256 digest of the repository identity, object
  format and complete ordered state vector;
- record projections read only from the direct commit OIDs in that vector; and
- the resulting structurally proved source selection for every work item and
  assembly.

`internal/gitx` may use private parser rows and prepared-transaction tokens, but
it must not expose a second snapshot or an alternate ref-resolution oracle.
Copies returned for diagnostics cannot mutate the Snapshot. Record reads,
materialization checks, action decisions and retry reconciliation consume the
captured vector; none re-resolves a ref ad hoc.

Exact capture uses one bounded `for-each-ref` batch including `%(symref)`.
Every requested ref omitted by the batch receives bounded literal
`symbolic-ref --quiet --no-recurse` and `show-ref --verify --quiet` probes.
Only both trustworthy “not symbolic” and “not present” results establish
genuine absence. Resolving and dangling aliases, direct non-commit refs,
unresolvable/broken loose or packed refs, malformed output, invalid UTF-8,
wrong-width OIDs, unexpected refs, duplicates, signals and untrustworthy exit
status retain distinct typed states and cannot be admitted as absence.

For Plan authority, target and release must be direct commits. A track ref may
be genuinely absent only while its release status is the exact pristine
baseline. A present owner must be a direct commit and prove one collective
materialization marker from the captured base, exact ordered frozen
dependencies, retained marker ancestry, status-only owner history, and
owner-ref authority. A release status supersedes an owner status only after
exact plan-ordered composition proves its `frozen_track_head` and topology.
Timestamps, completion order and later-looking divergent records are never
authority.

## Literal Git boundary and exact composition

`internal/gitx.Open` admits only a canonical absolute repository directory and
a canonical absolute executable regular file. Git is invoked directly with
literal argv and never through a shell or PATH lookup. Every process receives a
closed environment: fresh private HOME/XDG roots, `LANG=C`, `LC_ALL=C`,
system/global config disabled, replacement objects disabled, literal
pathspecs, user protocol disabled, prompting and pagers disabled, fsmonitor
disabled, an empty engine-owned hooks directory, and only explicitly required
index/object/identity variables. User aliases, hooks, filters, textconv,
credential helpers, attributes, environment injection and repository commands
cannot add behavior.

OID validation is format-bound at repository admission and shared by capture,
object reads, preparation and ref transactions. New and expected heads must be
existing commit objects of the repository’s SHA-1 or SHA-256 format. Refs are
canonical full branch refs; paths are canonical repository-relative UTF-8.

Product identity always excludes the fixed compiled record root
`.baton/releases` and no caller-supplied exclusion. Exclusion requires both a
structural record-only delta and a trusted commit-bound inertness decision.
Exact candidate OID, ordinary tree, ordered parents, ancestry, expected target
and record history remain separate mandatory evidence.

Record commits are prepared in an engine-owned temporary index from the exact
captured parent. Only action-derived regular `100644` paths below the fixed
record root may change. Commit identity, message and timestamp are
deterministic; the result is re-read to prove one parent, exact changed paths,
preserved record-root tree and unchanged product identity.

Composition is prepared without a working tree in an engine-owned bare/index
context using the admitted object directory. It is:

- the exact candidate for a true fast-forward;
- otherwise one deterministic two-parent commit with parents
  `[expected, candidate]`, a merge tree computed from those exact objects,
  fixed identity/message, and timestamp `max(parent timestamps)+1`; or
- a typed conflict/refusal.

Only built-in merge attributes are accepted. Custom drivers, external filters,
worktree/index/config state and model conflict resolution are forbidden. The
result is independently rechecked for exact parents, ancestry and deterministic
tree. Contained candidates, forged trees/parents, conflicts and stale expected
heads fail. Track composition, assembly preparation and release integration
use this same primitive; there is no alternate merge path.

## Exact CAS, crash and retry model

An action prepares one closed transaction from its admitted Snapshot:
repository/object-format identity, pre-Snapshot digest, unique sorted refs,
exact create/update/verify operations, the pre-state vector, desired direct
state vector, and the subset of meaningful operations. Verify-only and
update-to-same operations are non-meaningful. Public callers never supply a
ref, Git command, path, parent, message or merge mode.

Git runs `update-ref --no-deref --stdin` in prepared transaction mode. After
`prepare: ok` and while Git holds every exact named lock, the same capture
parser rechecks representation, existence, commit type, OID width and expected
pre-state. Any symbolic, broken, non-commit, unreadable or moved state sends
`abort`, waits for Git and returns without a ref move. Commit is sent only
after the complete vector matches.

The full transaction, including probes, has a hard 10-second monotonic
deadline. Protocol input/request and aggregate stdout/stderr are each bounded
at 512 KiB. Acknowledgements must be exact, ordered, valid UTF-8 lines with no
queued/trailing output. Start/write/read/EOF, malformed/missing/extra
acknowledgement, inspection, timeout, signal, non-zero exit and output overflow
are typed transport/protocol failures. A package-private fault seam exists only
in tests.

On every child outcome, including apparent success, post-commit failure and
lost acknowledgement, the parent closes pipes, aborts when still pre-commit,
terminates the transaction process group when necessary, waits exactly once
for every started process, removes the private hooks/home/index directories,
and recaptures the operation refs through the same Snapshot capture API.
Cleanup failure is retained as evidence but cannot override an authoritative
post-state classification.

Reconciliation has exactly three classes:

1. `desired`: every requested ref is a direct commit at the desired vector.
   This is typed success, including exact retry, post-commit exit/signal/
   timeout/output/protocol failure, acknowledgement loss, and pure verify.
2. `pre`: the transaction is meaningful and every meaningful ref exactly
   matches its captured direct/absent pre-state. This is typed
   snapshot-scoped `not-applied`; there is no internal retry. It explicitly
   states that the snapshot cannot disprove a transient commit-and-revert.
3. `ambiguous`: any mixed pre/desired vector, third OID, unexpected presence or
   absence, symbolic/broken/non-commit/unreadable state, capture failure, or
   vector mismatch. This is typed `recovery-required` and must never be
   automatically retried.

Desired is checked before pre, so pure verify remains success after
acknowledgement loss. Reflogs are diagnostic evidence only, never universal
transaction authority. The all-pre ABA limitation is explicit in the typed
outcome and proof.

`internal/baton` preserves these classes through its repository adapter and
stable errors/receipts. It maps `desired` to the pinned RC3 action receipt,
`pre` to a retryable only-after-fresh-Snapshot no-advancement outcome, and
`ambiguous` to recovery-required. It does not flatten transport failure,
staleness, conflict and ambiguity into one boolean and does not perform an
internal retry.

## Baton records, lifecycle and admitted actions

Plan and status parsing is byte-bound, closed and strict: duplicate keys,
unknown fields, trailing values, unsafe numbers, invalid UTF-8, wrong schema,
foreign release/work/track/refs, changed plan/approval bindings and malformed
optional sections fail. Lifecycle transitions match the pinned RC3 operation
goldens exactly, including role, stage/status/next-role, outcome, design,
Captain, proof, fresh Verifier, materialization, merge and blocker bindings.

The only public mutating facade remains:

```text
InstallApprovedPlan
ReboundPristinePlan
MaterializeTrack
RecordTransition
ComposeTrack
PrepareAssembly
IntegrateRelease
```

Each action locks its in-process writer, captures one fresh Snapshot, validates
the exact next responsibility and dependencies, resolves protected evidence
through the admitted resolver, prepares deterministic bytes/objects, performs
one exact CAS and returns the canonical receipt. Exact replay returns the same
receipt with `changed:false`; conflicting replay, consumed lifecycle,
out-of-order work, stale authority, unknown work/track, symbolic state or
foreign evidence fails without movement. Models never receive this facade and
never resolve conflicts or advance authority.

## Golden, adversarial and integration tests

`tools/batongolden` is development-only. Ordinary Go verification reads its
embedded corpus and never invokes Node, JavaScript, Git, a mutable checkout,
the network, HOME, or a user installation. The manifest binds the public RC3
tag object/commit/tree/support digest, exact `actions.mjs`, `git.mjs`,
`records.mjs`, and `transition.mjs` bytes, absolute Node/Git executables,
sanitized environment, object formats, oracle script digest, vector inventory,
sizes and SHA-256 values.

Explicit regeneration runs twice from two independently extracted, empty
RC3-bound source roots into two absent output roots with separate temporary
homes and caches. Both runs first reconstruct the tag/commit/tree/support and
`git.mjs` identities above. Complete inventories, modes and bytes must be
identical to each other before they may replace the checked corpus. The Go
implementation is never input to either JavaScript generation.

`internal/gitx/repository_test.go` ports and passes all eleven RC3 adversarial
exact-ref cases as named top-level tests:

1. capture refuses non-commit, resolving, dangling and broken exact refs while
   preserving genuine direct/missing controls;
2. exact create/update/verify reconcile and retry idempotently;
3. SHA-256 OID widths are preserved and inherited environment injection is
   ignored;
4. transaction/helper inputs are closed and bounded before effects;
5. resolving and dangling aliases refuse create/update/verify atomically;
6. all six after-capture resolving/dangling alias races for
   create/update/verify refuse beneath the prepared exact locks;
7. every pre-commit helper/acknowledgement fault aborts, waits and releases its
   lock;
8. post-commit transport and cleanup faults reconcile as idempotent success;
9. pure verify succeeds when transport is lost after commit acknowledgement;
10. every mixed or invalid reconciliation is ambiguous and is never internally
    retried; and
11. all-pre reconciliation is snapshot-scoped and does not claim ABA history.

The pre-commit matrix includes timeout, SIGKILL, early exit, missing/malformed/
extra acknowledgement, forced inspection failure and bounded stdout/stderr
overflow. Each row proves the complete pre-vector, transaction-child exit,
wait completion, no loose lock, removed private directories and a successful
follow-on CAS. The post-commit matrix includes non-zero exit, signal, distinct
timeout, extra/oversized output, parser failure, acknowledgement loss and
parent cleanup failure; each proves the desired vector and an exact retry with
no additional ref/reflog mutation. Ambiguity fixtures include mixed pre/post,
third OID, alias, unexpected presence/absence, broken direct ref, direct
non-commit ref and reconciliation failure, with exactly one helper attempt.

Additional tests cover:

- strict JSON, plan, status and every valid/invalid lifecycle transition;
- plan-derived paths/refs and closed public API/mutable-field surfaces;
- one complete SHA-1 and SHA-256 action delivery matching RC3 corpus bytes;
- materialization ownership, dependency transfer, divergent authority,
  selection without timestamp ranking and malformed/divergent refusal;
- protected approval and fresh read-only Verifier evidence bindings;
- record-only/product-inert history and mutation negatives;
- fast-forward/two-parent composition, custom-driver refusal, deterministic
  tree, conflict, forged topology/tree, stale target and exact integration;
- concurrent readers with one serialized action writer under `-race`; and
- exact replay for every admitted action.

## Budget and proof gates

The budget is a hard composition invariant, not a target:

- W0 currently has 6 runtime-production files and 673 physical lines.
- After W1, runtime production must have exactly 19 files and 7,190 lines.
  W1-owned `internal/baton` plus `internal/gitx` must be exactly 15 files and
  7,091 lines; the unchanged other four runtime files contribute 99 lines.
- Planned W2 contributes exactly 10 new runtime files and 3,274 net lines
  (including the `internal/driver/doc.go` change), producing exactly 29 files
  and 10,464 physical lines.

RC3 lines are paid for inside W1 by deleting the old duplicate ref parsers,
symbolic/existence probes, ad hoc resolution paths, repeated command-error
classification, duplicate prepared-state inspection, redundant OID conversion
and parallel retry logic. One capture parser, one bounded literal runner, one
prepared transaction classifier and one Baton adapter own those concerns.
Semantics, fault cases or typed outcomes must not be removed to meet the
budget.

Re-run the W0 byte-defined classifier in raw path-byte order. Emit, for W0,
post-W1, and a scratch composition of post-W1 plus the exact planned W2 patch:

- canonical `category<TAB>lines<TAB>bytes<TAB>sha256<TAB>path<LF>` manifests;
- runtime `path<TAB>lines<LF>` manifests;
- category file/line/byte totals and digests; and
- exact W1 and W2 deltas.

Tests, tooling, fixtures and generated Go are reported separately with exact
counts and deltas and cannot offset runtime growth. The scratch W2 composition
must use the pinned W2 product patch, remain disjoint from W1, apply cleanly and
produce 29/10,464; it is evidence only and is not committed by W1.

Proof must retain raw digest-addressed outputs for:

- start authority, plan/approval, materialization/dependency and ref inventory;
- RC3 tag/commit/tree/support/`git.mjs` reconstruction;
- old 26-path patch/path-list reconstruction and candidate path closure;
- both independent oracle generations and their byte-for-byte comparison;
- corpus verification and Go-versus-oracle comparisons for SHA-1/SHA-256;
- a case manifest and raw result for every one of the eleven adversarial tests,
  including PID/wait/lock/reflog/attempt evidence where applicable;
- focused race check
  `GOFLAGS=-buildvcs=false go test -count=1 -race ./internal/baton/... ./internal/gitx/... ./tools/batongolden/...`;
- non-race focused checks, `go vet`, `gofmt -l`, `go mod tidy -diff`,
  `git diff --check`, and candidate-only changed paths;
- W0, post-W1 and planned post-W2 line/category manifests;
- candidate commit, ordinary tree, product-tree identity, parent, exact
  product-only paths and clean worktree; and
- unchanged target/release/dependency/old-lineage refs before and after.

## Stop conditions

Stop without product implementation or ref movement on any of:

- stale/foreign status, changed plan/approval/design, missing current
  `PROCEED`, dirty start, non-record owner history, moved target/release/
  dependency/materialization, or changed product identity before edits;
- failure to reconstruct any pinned RC3 identity or either old-input digest;
- any old control artifact, path outside the exact 26, `.baton/releases`
  product change, module change, or caller-selectable Git/ref/path/merge
  surface;
- inability to establish genuine absence, direct-commit representation,
  sanitized absolute execution, prepared-lock recheck, full kill/wait/lock
  cleanup, or desired/pre/ambiguous reconciliation;
- any recovery-required ambiguity during implementation evidence;
- non-identical independent oracle generations, missing/non-PASS adversarial
  case, failed check/race test, or dirty evidence;
- post-W1 values other than 19 runtime files/7,190 lines, W1 surfaces other
  than 15/7,091, or planned post-W2 values other than 29/10,464; or
- product changes hidden after the candidate.

Any necessary change to outcome, authority, public action surface, safety
boundary, exact path closure, runtime file count, final line budget or planned
W2 composition requires a revised approved Plan; it is not an Implementer
discretion.
