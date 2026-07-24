# Approach

W1 will implement only the admitted authority kernel for plan
`sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8`.
This revision is selected from owner head
`4f3b726abfaf33085dd96af2a46cc4d36160419f`, where Captain review/1 returned
`REVISE` over design/1. The retained T1 materialisation marker is
`b2bb28778a4f73f5841bb936a993860d0b10d0ce`; its exact release base is
`48d640ba5cdd28f5f1f00e3cb58cec844d2f2a36` and its frozen T0 dependency is
`d00163283b33d384e5769a14da64ed30a1fad93a`. The implementation will not reuse
the retired v0.2 kernel or treat earlier design captures as authority.

The smallest implementation has two production layers and one development-only
oracle corpus, with no new module dependency:

1. `internal/baton` owns the complete Baton contract: immutable strict records,
   evidence admissions, captured authoritative projection and selection,
   work/track/assembly eligibility, candidate and record-history validation,
   lifecycle, retry reconciliation, receipts, and the sole seven typed
   actions. Its mutation surface is an opaque plan-bound `Actions` value with
   exactly `InstallApprovedPlan`, `ReboundPristinePlan`, `RecordTransition`,
   `MaterializeTrack`, `ComposeTrack`, `PrepareAssembly`, and
   `IntegrateRelease`.
2. `internal/gitx` is a Baton-agnostic mechanical dependency. It knows no plan,
   approval, evidence, lifecycle, owner, work, track, assembly, record-root
   policy or action. It executes sanitized Git and exposes only typed,
   captured-OID reads, tree/product hashing primitives, deterministic
   blob/tree/commit or composition preparation, and an atomic exact-ref
   transaction. `internal/baton` owns the narrow repository interface it
   consumes and alone translates the admitted plan into refs, paths, commit
   metadata, parents and compare-and-set operations. There is no raw command
   runner or caller-selected merge mode.
3. `tools/batongolden` owns checked-in, development-only vectors emitted by the
   pinned JavaScript reference. Go tests consume those bytes without Node,
   the user installation, the network, or the JavaScript reference at test or
   runtime.

`Plan` and `Status` will have private state and copy-returning accessors. Only
`ParsePlan` can mint an admitted plan: it requires the exact fence at byte zero,
digests the complete raw file, owns a private copy of the bytes, and validates
the complete metadata graph. Only strict parsing can mint a status. Callers
therefore cannot construct a trusted value by filling an exported struct or
mutate one after validation. Typed errors retain the stable RC2 error code so
goldens can compare behavior without depending on prose.

Structural status validity remains separate from trusted action admission.
The `internal/baton` action constructor receives an explicit `guided` or
`autonomous` profile, one implementation of its consumer-owned `Repository`
interface, and two trusted engine callbacks:

- the evidence resolver returns exact approval or Verifier-dispatch bytes plus
  closed provenance; the kernel checks the raw digest, protected ref,
  approved plan, isolated/non-writable authorizer, and, where present, the
  exact fresh, read-only, engine-controlled Verifier invocation, proof,
  candidate and product tree;
- the behavioral-inertness resolver returns one exact decision for one
  repository, `.baton/releases`, and immutable commit OID.

Evidence results are copied and cached by exact request inside one `Actions`
instance. Opaque evidence admissions bind the full immutable status and
profile. A parsed status, copied board row, boolean claim, or inertness decision
for another commit is insufficient. Missing, asynchronous, malformed,
`consumed`, or otherwise unknown resolver output fails closed.

The consumer-owned repository interface is deliberately mechanical and closed:
capture an ordered set of exact head refs; inspect object format; read one or a
bounded batch of blobs at a captured OID; enumerate a captured tree; read
parents, ancestry and changed paths for exact OIDs; calculate a tree identity
with an explicit exclusion prefix; prepare a deterministic tree/commit or
composition from a closed value request; and execute one closed atomic
create/update/verify ref transaction. Its implementation is `internal/gitx`.
Requests contain typed refs, canonical repository paths, exact OIDs, bytes and
fixed commit metadata, never argv, environment, shell text or callbacks. All
Baton meaning and admission occurs before `internal/baton` makes one of these
mechanical calls.

W1 also exposes an explicit read-only Baton projection for W3. A
plan-bound `Reader` captures the plan refs once and returns an opaque immutable
`Snapshot`. The snapshot provides:

- `SelectWork(workID)` and `SelectAssembly()` with the exact selected
  baseline/owner/composed source, captured ref/OID, strict status and handoff
  digests;
- `NextWorkForTrack(trackID)` and `MayAdvanceWork(workID)` for serial work
  eligibility;
- `MaterializationFor(trackID)`, `TrackReadyForComposition(trackID)` and
  `ReleaseReadyForAssembly()` for exact dependency and collective gates.

These methods return only Baton domain values and typed structural errors.
They do not resolve trusted evidence, create an admission, expose record paths
or repository mutation, retain a repository callback, or define workers,
leases, retries, effects, journal rows or scheduler state. An action always
captures and validates fresh authority again, so a W3 scheduling snapshot is
never mutation authority.

The `internal/gitx` boundary will execute an explicitly configured absolute,
regular, real-pathed Git binary
directly, never through a shell or inherited `PATH`. Every invocation uses a
closed environment with C locale, temporary home/config/hooks/index state,
literal pathspecs, terminal prompts and pagers disabled, system/global config
disabled, replace refs disabled, fsmonitor disabled, and only the narrowly
required author/committer or engine-owned object-directory variables. Reads use
full captured commit OIDs, NUL-delimited output, fatal UTF-8 decoding for paths,
closed path/ref validation, and the RC2 byte/count/history ceilings. Repository
opening fixes one explicit `sha1` or `sha256` object format, and every later OID
must have the matching 40- or 64-lowercase-hex length. The generic tree reader
reports exact modes/types; `internal/baton` uses those facts to reject its fixed
record root if any launch-worktree or captured-tree component is a symlink or
non-tree.

`internal/baton` selects authority from one bounded repository snapshot of the
target, release and all planned owner refs. The release projection and each
present owner projection are read at those exact OIDs. An absent owner admits
only a pristine release baseline. A present owner must share one
materialisation, begin with the exact collective record-only marker, retain
that marker in release ancestry, and use its planned owner ref. Owner state
remains authoritative until the release projection proves a collective
completed transfer bound to that exact frozen owner head. Missing or malformed
owner records, partial transfers, foreign copies, erased markers, timestamps,
and “newest” guesses never select authority.

`internal/baton` candidate admission replays first-parent history from the
exact materialisation base for the first work, or the preceding work's exact
passed candidate for later work. It will:

- require the proof repository, base, candidate OID, ordinary tree OID and
  product-tree digest to match Git and require the candidate to be reachable
  from the captured authority head;
- classify every commit as product-only or record-only and reject mixed
  commits, cross-work record commits, unexpected record paths, non-regular
  handoffs, out-of-order transitions, product before current Captain
  `PROCEED`, later work before earlier `PASS`, scope/touch-surface escape, an
  empty candidate, or a final candidate commit containing records;
- replay each recorded status change through exactly one admitted lifecycle
  result and validate exact design/proof bytes at the captured ref;
- allow only direct, single-parent, structurally record-only commits after the
  candidate, for that work's status/design/proof paths, and reject any hidden
  product change.

For product identity, `internal/baton` first resolves an exact `inert` policy
admission for the candidate OID, then asks the Baton-agnostic repository
primitive to inventory the exact recursive tree and exclude its supplied
`.baton/releases` prefix. `internal/gitx` sorts entries by raw UTF-8 path bytes
and hashes for each entry:

```text
path NUL mode NUL type NUL object LF
```

This preserves executable, symlink, submodule and blob identity. Equal product
digests may establish that a direct record transition preserved product, but
never replace exact commit/tree identity, ancestry, candidate history,
authority reachability, or expected-target comparison.

The lifecycle validator will implement the RC2 closed transition table for
`DESIGN_WRITTEN`, `PROCEED`, `REVISE`, `ESCALATE`, `IMPLEMENTED`, `PASS`,
`FAIL`, `BLOCKED`, `MERGED`, `MATERIALIZE`, `REBOUND`, and `NO_VERDICT`.
It preserves immutable gates, requires refreshed design/proof and producer
identity after `REVISE`/`FAIL`, enforces distinct Captain and Verifier
invocations, makes terminal status write-once, permits byte-unchanged
redispatch only for `NO_VERDICT`, and keeps assembly `FAIL`/`BLOCKED` routed to
Planner without inventing an in-place repair transition.

Each `internal/baton` action snapshots its dedicated Go input and all handoff
bytes before the first repository read, derives every
ref/path/message/topology choice from the admitted plan, asks the mechanical
repository interface to prepare immutable commits, validates their prospective
plan-bound snapshot, then supplies one exact compare-and-set transaction and
re-reads installed heads. Record commits use one exact parent, `100644` blobs
in an engine-owned temporary index, fixed `Baton Records
<records@baton.invalid>` author/committer identity, parent timestamp plus one
second, and one of the plan-derived messages `Install approved Baton plan
<release>`, `Rebound pristine Baton plan <release>`, `Record <scope>
<identity> <result>`, `Materialize Baton track <track>`, `Transfer composed
Baton track <track>`, `Prepare Baton assembly <release>`, or `Integrate Baton
release <release>`. Callers cannot supply or alter those bytes.

- `InstallApprovedPlan` requires an existing target, absent owner refs and an
  absent release namespace; it creates the release ref at a deterministic
  record-only commit containing exactly `plan.md` and every pristine baseline
  status. An existing release is accepted only as the exact canonical retry.
- `ReboundPristinePlan` requires an admitted previous plan with the identical
  release ownership topology, no owner ref, and exact predecessor/result
  namespaces containing only the plan and all baselines. It changes plan,
  approval and permitted target binding without carrying lifecycle progress.
- `RecordTransition` accepts only a work identity plus ordinary result and
  immutable next status, or an assembly result with no work identity. It checks
  serial eligibility, both evidence admissions, exact changed handoff bytes
  and candidate/assembly history, then advances only the selected authority
  ref while verifying every other captured plan ref.
- `MaterializeTrack` derives the current release base and ordered frozen
  dependency heads, changes every work status in the track together, and in
  one transaction advances release and creates the owner ref at the same
  marker while verifying target and every other owner.
- `ComposeTrack` requires every work passed on the exact frozen owner. It
  prepares the exact release/owner composition, then one direct record-only
  transfer commit that moves all work statuses to release authority together;
  only the release ref advances and all other refs are verified.
- `PrepareAssembly` requires every exact transfer, derives the ordered
  component heads, and records proof plus `verify / ready / verifier` status
  in one direct release record commit. Proof base and candidate are both the
  exact pre-preparation release head.
- `IntegrateRelease` requires the exact assembly `PASS`. It prepares target
  composition and the terminal release status, then atomically advances target
  and release while verifying every track owner.

Exact retries do not trust a matching projection. They re-read the one-parent
predecessor, re-run eligibility, evidence, handoff, candidate/assembly history
and prospective-state validation, reconstruct the canonical commit or
composition OID, and return `changed:false` only when it is the original exact
effect. A copied status, structurally equivalent sibling commit, changed
predecessor, stale ref, partial effect, or already-consumed state is an error.
Receipts are opaque immutable Go values whose accessors and JSON marshaling
return copies of engine-owned JSON-only data.

Composition is deterministic and conflict-free. After `internal/baton`
establishes lifecycle eligibility and derives the exact expected/candidate
OIDs and fixed metadata, `internal/gitx` mechanically returns the only possible
mode. If expected is an ancestor of candidate, the result is that exact
fast-forward. If candidate is already contained by expected, preparation
fails. Otherwise an engine-owned bare context and temporary index compute
`merge-tree --write-tree --no-messages` with ordered inputs
`expected, candidate`. Both trees are checked for merge attributes; custom
drivers are rejected before execution, while only Git's built-in
unspecified/text/binary/union semantics from the expected side are installed
in the isolated context. A conflict is an error. The two-parent commit uses
ordered parents `[expected, candidate]`, the computed tree, the supplied fixed
Baton Merge identity, timestamp `max(parent timestamps)+1`, and the supplied
plan-derived message. The primitive accepts no merge-mode parameter.
`internal/baton` independently recomputes and binds the returned tree/parents/
OID in its prospective transition, rejecting a forged tree, reversed/extra
parents, or noncanonical sibling.

The JavaScript oracle generator under `tools/batongolden` will refuse any
reference root unless these exact RC2 source digests match:

- `actions.mjs`:
  `sha256:dd450cbf7073dd7979c7ea74b806c5555169c0defb4dd941923933dd35dc8f78`
- `git.mjs`:
  `sha256:6e41ea115f06580d1a0415e65a4ff4d5dd4568842a168fd89815c06ab76b56b2`
- `records.mjs`:
  `sha256:447e0277e3f088578427cc00828c5da0f83f828238b72b639d4e9aca42772c84`
- `transition.mjs`:
  `sha256:87e89368749516cfefe9b5e0735a5d29089c28ed0edc117fb340d48d51241fa3`

It also binds Baton tag commit
`890238ef063bb53cf51fb3359f1ff527f14846c6`, tree
`97513f3e6f798f3ad04d5b510a49496a605a8ea4`, and support digest
`sha256:676c630c6a4ef3f752d604efaa5e51958adec0d8580b74cec7fb1e689b1d3436`.
The generator imports only that JavaScript reference, builds deterministic
disposable Git repositories, invokes the safe seven-action facade, and emits
canonical JSON cases plus a path/size/digest manifest. It never imports,
executes, or reads Go output.

Fixture generation forbids `PATH` lookup and requires `--git` to name an
absolute executable. It resolves and records that real path, `git --version`,
the closed sanitized-environment profile, and the exact object format in the
corpus provenance. The checked-in manifest pins that observed absolute real
path; reproduction passes it explicitly and fails if it is unavailable rather
than falling back to `PATH`. Both the JavaScript oracle and Go reproduction are
configured with that absolute Git path; hostile inherited
`PATH`, config, hooks, attributes, replace refs and fsmonitor settings remain
canaries rather than inputs.

Every repository recipe fixes branch refs, file bytes and modes, merge
attributes, author and committer names/emails, integer timestamps, time zones,
parent order and commit messages. It creates commits through plumbing rather
than ambient user config or wall-clock `git commit`. The corpus is a matrix
over repositories initialized explicitly with
`--object-format=sha1` and `--object-format=sha256`. Each case records
`object_format`, `oid_hex_length`, every input OID and every expected OID;
40-hex values are accepted only in the SHA-1 cases and 64-hex values only in
the SHA-256 cases. Parser vectors also reject truncated, uppercase, mixed-width
and format-mismatched OIDs. Product identity, fast-forward and two-parent
composition, ref transactions, complete seven-action delivery and exact retry
run in both formats.

Checked-in groups cover strict records, lifecycle, product identity,
owner/history selection, composition and a complete seven-action release.
`batongolden verify` validates the admitted package and the complete corpus
manifest. Ordinary Go tests use the checked-in corpus only; regeneration is
explicit and is never part of build, test or runtime startup.

# Surfaces

- Product changes in `internal/baton`: retain `release.json`,
  `snapshot/**`, and their admission identities byte-for-byte; add the strict
  parser/writer, immutable plan/status types, stable errors, semantic binding,
  evidence admission, plan-derived paths, immutable read-only
  snapshot/selection/eligibility API, authority and candidate-history
  validation, lifecycle/reconciliation, opaque receipts and all seven typed
  actions. Existing asset admission may share the new strict decoder only if
  all W0 mutation tests remain unchanged.
- Product changes in `internal/gitx`: replace the documentation-only seam with
  the Baton-agnostic sanitized executor, object-format-bound OIDs, exact head
  capture, captured-OID blob/tree/graph reads, generic product-tree identity,
  deterministic tree/commit/composition preparation and atomic
  create/update/verify ref transactions. It adds no Baton record, evidence,
  authority, lifecycle, eligibility, receipt or action type.
- Development changes in `tools/batongolden`: extend `verify`, add the
  reference-only oracle generator, and add the digest-manifested RC2 vector
  corpus.
- Tests in `internal/baton`: focused strict-JSON, plan graph, status semantics,
  raw-digest, immutability/aliasing, evidence-provenance, read-only W3
  projection, authority/eligibility, candidate history, lifecycle, each typed
  action, exact retry and golden parity tables. External-package integration
  tests instantiate the real `internal/gitx` implementation without moving
  Baton policy into it.
- Tests in `internal/gitx`: real temporary-repository tests for product
  identity, exact captured-OID reads, SHA-1/SHA-256 validation, fixed-metadata
  commit preparation, deterministic composition, atomic ref transactions and
  hostile Git/ref/path/environment behavior. Tests contain no plan, approval,
  work, lifecycle or action fixture.
- Tests in `tools/batongolden`: source/corpus manifest closure, deterministic
  verification output, missing/extra/changed vector rejection, and CLI
  invocation closure.
- Configuration changes: none. `go.mod`, `go.sum`, CI, AGENTS instructions and
  command/runtime configuration remain untouched; the implementation uses the
  Go standard library and the trusted Git executable already required by the
  repository.
- Baton records: the product candidate will not edit `.baton/releases`.
  Normal Baton implementation actions later record W1 design, proof and status
  separately. The delivered kernel recognizes and writes only the standard
  plan-derived record paths and never exposes those records as product,
  workspace/model/check/build/package input.
- Explicitly untouched: `cmd/sworn`, `internal/runtime`, `internal/journal`,
  `internal/driver`, adapters, conformance process adapters, cockpit,
  observability, docs captures, and every path outside the W1 include set.

# Consequential decisions and risks

- **Package direction and safe API:** `internal/baton` imports the
  Baton-agnostic `internal/gitx` implementation and owns the consumer
  interface, read projection and action facade. The risk is leaking generic
  mechanics upward as alternate authority; API-shape tests prohibit argv/raw
  mutation helpers, and W3 receives only `Reader`/`Snapshot` plus the seven
  typed actions.
- **Read projection staleness:** W3 may schedule from a snapshot after a ref
  moves. Snapshot values remain honest because they bind exact OIDs, but never
  authorize mutation; every action recaptures authority and stale work loses
  eligibility or compare-and-set.
- **Strict parser parity:** Go's stock JSON decoder is not sufficient for
  duplicate names, lone surrogates or reference number behavior. A small
  bounded parser/writer is more code, but avoids a dependency and is tested
  against independently generated positive, mutation-negative and resource
  vectors.
- **Immutable inputs in Go:** slices, maps and callback results can otherwise
  alias caller storage. Public records expose no mutable fields; constructors,
  action entry points, resolvers, handoffs and receipt accessors copy at the
  boundary, with aliasing and race tests.
- **Record/product separation:** excluding records is safe only while a trusted
  resolver proves this exact commit inert. There is no permissive default.
  Unknown or consumed policy stops before ref mutation; product equality alone
  never admits a candidate or retry.
- **Ambient Git behavior:** hooks, config, replace refs, fsmonitor, path
  quoting, custom merge drivers and inherited environment can alter facts or
  effects. One explicitly absolute literal executor plus adversarial canaries
  closes these inputs. Repository object format is captured once and exact OID
  width is enforced thereafter; unsupported format or merge capability fails
  closed.
- **Composition portability:** built-in Git merge semantics can expose
  version-sensitive behavior. The implementation uses the same plumbing
  operations as RC2, checks exact golden trees/OIDs for simple fast-forward and
  divergent cases, and treats any conflict or different canonical result as
  an error rather than accepting a sibling.
- **Prepared unreachable objects:** Git may create immutable blobs, trees or
  commits before the final compare-and-set loses. Authority refs remain
  unchanged, receipts do not claim success, and exact retry reconciliation
  ignores unreachable objects. W3, not W1, owns durable effect attempts and
  crash scheduling.
- **Bounded history cost:** complete candidate and marker replay is required
  for trust. RC2 byte, path, ref and 10,000-commit ceilings stop hostile input;
  batched reads avoid one Git process per status. No cache becomes a second
  authority.
- **Archaeology isolation:** v0.2 contains superficially similar policy,
  repository and store code, but its contracts predate RC2. No source is
  copied; every production behavior must be justified by the pinned reference
  or admitted plan and exercised by a focused test.
- **Deliberate deferral:** the kernel provides deterministic synchronous
  actions and exact retry reconciliation only. It adds no scheduler, SQLite
  journal, leases, invocation attempts, driver ABI, process isolation,
  adapter, board, recovery loop or speculative framework; those remain W2/W3
  or later work.

# Evidence plan

- **A-W1-authority / independent oracle:** the corpus manifest records the
  exact RC2 tag/commit/tree/support and four JavaScript source digests above,
  absolute real-pathed Git executable and version, sanitized-environment
  profile, fixed repository/commit recipe, explicit object format, every vector
  path/size/digest, and deterministic generator version. Two explicit oracle
  runs in separate temporary directories for both `sha1` and `sha256` must
  produce byte-identical format-specific corpora. Go tests reject changed,
  missing, duplicate, extra, wrong-width or cross-format vectors. No Go test
  invokes Node or searches `PATH`.
- **A-W1-authority / strict records and bindings:** table tests consume all
  seven admitted raw strict-JSON fixtures plus invalid UTF-8, trailing input,
  hostile names, size/depth/list limits and single-field plan/status mutations.
  Positive work and assembly fixtures match the oracle's canonical values and
  digests; negative cases assert the same stable RC2 error code for fixed root,
  ref/path, graph, projection, stale plan/approval/design/Captain/proof/
  candidate/product/Verifier/Merge and self-review failures.
- **A-W1-authority / lifecycle:** golden tables exercise the full work path,
  `REVISE` with a fresh design, `ESCALATE`, `FAIL` with a fresh proof,
  `BLOCKED`, assembly `PASS`/`FAIL`/`BLOCKED`, `MATERIALIZE`, pristine
  `REBOUND`, unchanged `NO_VERDICT`, and terminal write-once behavior. Every
  non-admitted result and one-field gate mutation fails.
- **A-W1-authority / read-only W3 contract:** compile-time external-package
  tests use only `Reader.Capture`, immutable work/assembly selection and the
  four eligibility queries. Mutation, repository handles, record paths,
  evidence admissions and runtime/journal fields are absent. A ref moved after
  capture leaves the old snapshot exact while the next action independently
  rejects its stale authority.
- **A-W1-authority / product and history:** deterministic repositories cover
  ordinary tree plus ordered path/mode/type/object product digest, executable,
  symlink and submodule entries, record-only equality, product inequality,
  equal-product but wrong ancestry, wrong candidate/tree, consumed/unknown
  exclusion, symlinked root, product-after-candidate, mixed commit, cross-work
  record commit, product-before-`PROCEED`, out-of-order work, scope escape and
  a non-product final candidate. The identity and ancestry table runs with
  exact 40-hex SHA-1 and 64-hex SHA-256 objects.
- **A-W1-authority / authority selection:** real-Git cases progress baseline to
  exact owner to proven collective transfer. Missing owners, foreign/newer
  copies, erased markers, divergent materialisations, unmet or reordered
  dependency heads and partial transfers fail with captured refs unchanged.
- **A-W1-authority / all seven actions and retries:** one oracle-bound
  autonomous scenario per explicit object format invokes every
  `internal/baton` action through work and fresh assembly `PASS` to exact
  integration and compares canonical statuses, handoff digests, receipts,
  commit OIDs, trees and final refs. Each action is immediately retried and
  must return the exact `changed:false` receipt with no new commit or ref
  movement. Copied post-states, noncanonical sibling commits, stale
  predecessors and advanced states must fail reconciliation.
- **A-W1-authority / Git and composition:** fast-forward and ordered
  two-parent cases compare the exact oracle tree and result OID in SHA-1 and
  SHA-256 repositories built with fixed metadata through the pinned absolute
  Git path. Conflict, candidate-already-contained, forged tree, reversed/extra
  parents, custom merge driver, hostile `PATH`/config/hook/fsmonitor/
  replace-ref, malformed NUL/path output, OID-width mismatch and unsupported
  capability cases fail closed. Contention at install, record,
  materialisation, composition and integration snapshots all relevant refs
  before and after and proves no partial ref advancement. Package/API tests
  also prove `internal/gitx` contains no Baton plan, evidence, lifecycle,
  authority, action or receipt policy and exports no argv runner.
- **Required check:** run exactly
  `GOFLAGS=-buildvcs=false go test -race ./internal/baton/... ./internal/gitx/... ./tools/batongolden/...`
  from a clean product candidate. Retain focused verbose outputs for the
  strict-record corpus, complete seven-action loop, candidate-history
  adversarial table, product identity table, deterministic composition table
  and compare-and-set contention table, then record their raw evidence
  digests in W1 proof.
- **Scope and separation:** inspect the committed candidate range and require
  every product path to be under `internal/baton`, `internal/gitx` or
  `tools/batongolden`; require its final commit to be product-only; verify no
  Node/JavaScript asset is linked into the Go kernel or binary and no runtime,
  journal, driver, adapter or cockpit symbol was introduced. The later W1
  proof/status record commit must preserve the candidate product-tree digest.

# Revisions

Design/2 after Captain
`codex:/root/w1_authority_captain/sworn-v0.3.0/T1-authority/W1-authority-core/review/1`
returned `REVISE` over design/1. Producer invocation:
`codex:/root/w1_authority_implementer/sworn-v0.3.0/T1-authority/W1-authority-core/design/2`.

- Moved strict records, evidence, authoritative selection/eligibility,
  candidate validation, reconciliation, receipts and all seven typed actions
  into `internal/baton`.
- Reduced `internal/gitx` to a Baton-agnostic implementation of the narrow
  consumer-owned repository interface: captured-OID facts, product/tree/commit
  preparation and atomic ref transactions only.
- Added the immutable read-only Baton `Reader`/`Snapshot` selection and
  eligibility contract needed by W3, without mutation or runtime/journal
  types.
- Made deterministic evidence bind an explicitly absolute sanitized Git
  executable, fully fixed commit metadata and explicit SHA-1/SHA-256 fixture
  matrices covering RC2's 40- and 64-hex OIDs.

All other design/1 lifecycle, retry, record/product separation, composition,
adversarial evidence and W1 scope boundaries remain. The design is derived
from the admitted RC2 plan, compiled conformance/driver assets and
digest-pinned JavaScript record reference; earlier Sworn captures and v0.2
history remain archaeology only.
