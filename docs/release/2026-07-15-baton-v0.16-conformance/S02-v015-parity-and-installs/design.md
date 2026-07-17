# Design TL;DR — S02-v015-parity-and-installs

**Slice:** `S02-v015-parity-and-installs` · **Track:** `T1-foundation` · **Release:** `2026-07-15-baton-v0.16-conformance`
**State:** `in_progress` — implementation is committed through `60dcd6291ddbe3491e16a05d2ed98d896d714165`; maintainability and real-home sync remain paused for this replan
**Covers:** `N-01`, `N-02`, `N-10` · **Effort/complexity:** high / high (`beast`) · **Boundary:** ratified 47-file bootstrap exception

## User outcome

The built `sworn` binary, the committed normative repository content, and both
supported user-level Baton installations agree on one authority: Baton
`v0.15.1`, upstream commit
`3fb4d275ae8a151f6287e7b9279d71628b12eea0`, source digest
`sha256:8f0839ea897374eb10d6db2a789939714727739621babef1117d74cbf4488d2f`,
and upstream root `VERSION` bytes exactly `v0.15.1` plus LF at blob
`5f1dd0af59642311ee04e018a0023562d4dde008`. That upstream object is not
Sworn's `internal/adopt/baton/VERSION`: the latter remains a separate
multi-line adopting-repository manifest whose committed blob is resolved on
every participating ref and whose parsed tag/SHA/digest must agree across those
refs, with the marker, and with the binary. Any mapped byte, schema
classification, archive identity, native managed-tree byte/mode, installed
tree, or Sworn sentinel mismatch fails closed. Repair either installs both
supported mirrors exactly or retains complete owner-only recovery authority
until all three logical roots have been restored to their pre-run state.

## Readiness verdict

- S01 is `verified`; its vendor plan already materialises candidates before the
  first mutation and protects mapped files plus VERSION with one snapshot,
  apply, rollback, and restart-recovery transaction.
- The amended S02 spec has five concrete, traced EARS criteria. It explicitly
  owns the embed, public diff, expanded repository transaction, archive/native
  generation, three-root install transaction, thin CLI adapters, fixtures,
  bounded opaque status carrier, and binary reachability tests required by C-01.
- The archive operation independently reproduces 78 entries, SHA-256
  `27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15`,
  and Git blob `39ae650dfe0282b0fa8bda14e1a01e7084077702` at the pinned commit.
- The ratified Coach correction gives `upstream_version_blob_oid` exactly one
  meaning, fixes both script oracles to `umask 0022`, and requires physically
  disjoint install/recovery roots plus complete durable recovery authority
  before the first replacement. No Captain pin remains for the Implementer to
  interpret locally.
- `status.json` remains `in_progress` with immutable `start_commit`
  `e61cb190736ee7483fb4ed1a993442b26ce3574c`, pending cycle 0, an empty
  maintainability ledger, and no adjudication. The repository-wide gate exposed
  the five-path scope defect after the prior Captain review; only the amended
  carrier/fixture delta requires a new `PROCEED` before implementation resumes.

## Proposed implementation

1. **Make one complete archive candidate before repository mutation.** Resolve
   the exact clean `v0.15.1` checkout and C-01 tag/SHA/digest. Require the
   upstream tree's root `VERSION` to be the regular blob
   `5f1dd0af59642311ee04e018a0023562d4dde008` with complete bytes
   `v0.15.1\n`; only that object is `upstream_version_blob_oid`. Then have
   `internal/baton/installer_archive.go` invoke the argv-equivalent pinned
   `git archive` operation and validate the complete tar SHA-256, Git blob OID,
   78-entry path/mode/blob inventory, prefix, canonical names, and upstream
   object identities before exposing immutable candidate bytes.
2. **Expand S01's repository transaction, not its authority model.** Extend the
   vendor plan in `vendor.go` so mapped bytes, the generated multi-line adopting
   manifest candidate, and the validated archive candidate are all materialised
   before snapshots.
   Feed the byte-sorted combined candidate set to the existing
   `vendor_transaction.go` apply/verify/rollback machinery. Extend its recovery
   allow-set and tests to include exactly the archive destination; a direct tar
   write or separate recovery record is forbidden.
3. **Vendor only the mapped v0.15.1 delta and keep both VERSION identities
   separate.** Add
   `schemas/spec-ambiguity-report-v1.json` to the explicit mapping, dedicated
   schema embed, fixture, and grade classification. Preserve every normative
   JSON byte verbatim; apply only the already documented Markdown command/path
   transforms. Build `internal/adopt/baton/VERSION` through S01's existing
   multi-line pin-candidate machinery; never replace it with upstream
   `v0.15.1\n`. For each participating ref derived by the proof/operation,
   resolve that committed path through Git as a regular blob, compare manifest
   blob identity across participants, and separately parse and compare its
   `baton-protocol`, `upstream-sha`, and `upstream-digest` to the marker and
   embedded binary pin. Run the repaired upstream vendor transaction once, then
   require a second `vendor --check` and public `baton diff` to converge with
   zero drift. S05 retains ownership of general operation/ref policy; S02 does
   not invent a caller-selected participant set.
3a. **Keep exact-schema consumers honest without taking lifecycle authority.**
   Add one optional opaque/lossless `maintainability` carrier to `state.Status`
   so an explicitly supplied v0.15 object survives `Read` and `Write`. Do not
   default it, interpret it, validate transitions, migrate records, or change
   `StartCommit` semantics. Update only canned generic-check and state/run test
   fixtures to supply their own canonical `check`, `start_commit`, and
   maintainability facts. Retain every exact-schema assertion. S03 still owns
   the complete typed/null carrier and atomic writers; S04 still owns requested
   check matching and retired-maintainability dispatch behavior.
4. **Give the archive one explicit binary owner.** Add
   `internal/adopt/baton_archive.go` with the sole `go:embed` directive for
   `baton/installer-input-v0.15.1.tar` and a read-only byte accessor. Production
   consumers use those embedded bytes only: no repository-path fallback, live
   Baton checkout, network fetch, or runtime shell is accepted.
5. **Keep public archive parity separate from ordinary mapped content.** Extend
   `internal/baton/diff.go` to consume the archive validator and compare the
   repository archive, compiled embed, exact inventory/modes/blobs, upstream
   root VERSION identity, and the separately resolved adopting manifest
   identity/parsed pin. `content.go` remains unchanged and continues to own only
   ordinary mapped-file/rules materialisation. Deterministic drift returns
   public exit 1; malformed mapping/archive or operational failure returns 2.
6. **Generate both complete managed trees natively.** In
   `installer_archive.go`, safely parse the validated tar with Go stdlib
   `archive/tar`; reject traversal, absolute/non-canonical names, links, devices,
   special files, duplicates, invalid UTF-8, unexpected modes/types, or
   missing/extra inventory. Generate staged Claude and Codex trees using the
   exact tagged installer semantics, including Codex frontmatter reconstruction,
   argument-resolution prelude, and the documented Markdown/JSON path rewrites.
   Every created directory is explicitly fixed and re-verified at `0755` and
   every regular file at `0644` after creation, so the native result is
   independent of the process's inherited umask. The shipped path invokes no
   shell or installer script.
7. **Derive and assert the exact eight-command set.** Inventory comes from the
   validated archive and must equal, in unsigned-byte order:
   `design-review.md`, `implement-slice.md`, `mark-shipped.md`,
   `merge-release.md`, `merge-track.md`, `plan-release.md`,
   `replan-release.md`, and `verify-slice.md`. Claude receives all eight command
   files; Codex receives all eight corresponding `baton-*/SKILL.md` wrappers.
   Stale installer prose that says seven is not a second authority.
8. **Prove native output against two fixed-umask script oracles.** Tests extract
   the exact embedded archive into isolated proof directories and separately run
   its pinned `install-codex.sh -y` and `install-claude.sh -y` through an oracle
   launcher that explicitly sets `umask 0022`, with empty temporary `HOME`,
   `AGENTS_HOME`, `CODEX_HOME`, and `CLAUDE_HOME`. Compare every managed path,
   mode, and byte to native output. A negative fixture starts the launcher under
   hostile inherited umask `0077` and proves the inner `0022` plus native
   post-create mode fixing still yield directories `0755` and files `0644`.
   Neither script shares native generation logic, and success on only one mirror
   is failure.
9. **Reject unsafe root topology before any mutation.** In
   `internal/baton/install_transaction.go`, physically resolve the existing
   prefix and every component of logical `agents_home`, `codex_home`,
   `claude_home`, and the recovery root before creating a snapshot, staging
   target, or directory. Reject a symlink at any component, unsupported special
   node anywhere beneath a target, invalid UTF-8, and roots that are equal,
   nested, ancestor/descendant, canonical-path aliases, inode aliases, or overlap
   recovery. Missing suffixes are joined only to a physically resolved existing
   ancestor and participate in the same four-root pairwise-disjoint check.
10. **Repair all roots under recovery authority durable before replacement.**
    Stage complete pre-run-preserving target trees for the three logical roots,
    add the canonical outputs and separate Sworn VERSION sentinels, and snapshot
    every complete pre-run tree. Publish the transaction directory, recursive
    snapshots, exact `manifest.bin`, and fixed sentinel with `0700` directories
    and `0600` files; fsync files and containing directories, atomically publish
    and re-read the sentinel, and only then perform the first whole-root
    replacement. Replace and verify all three roots in one transaction.
    Sentinel presence always routes a later invocation to recovery-only exact
    restoration; it may not generate or install a new canonical tree. Failed
    restoration durably updates only the unique unsigned-byte-sorted logical
    `unrestored_paths`, retains complete authority, and exits 1. Exact recovery
    verifies all pre-run roots, retires authority, and exits 2 with rerun
    guidance. Normal success retires and directory-syncs authority only after
    all three installed roots verify. `cmd/sworn/doctor.go` only resolves input
    paths, renders path-only results, and maps outcomes to exits.
11. **Freeze and prove the complete C-01 boundary.** Exact parity tests enumerate
    the live mapping/schema map rather than restating it, mutation-test every
    upstream-VERSION/adopting-manifest/archive/mapping/schema/native/install
    layer, and pair verdicts with 0/1/2 exits. Fault/kill tests cover every
    durable publish, per-root replace, per-root verify, rollback/recovery step,
    and authority-retire boundary. After deterministic checks and proof are
    committed and clean, run the exact planning-authority Gate-8 Implementer
    preflight; the later fresh Verifier remains the only authoritative
    certification.

The crash-state oracle is explicit:

| Last durable boundary | Restart behavior |
|---|---|
| Before sentinel publication | No root was replaced; validate/clean only incomplete owner-only staging, then require a fresh invocation. |
| Sentinel durable, before/after any root replacement or verification | Recovery-only restoration of all three complete pre-run roots; no new generation or install. |
| Any rollback or recovery step fails | Retain the complete snapshot/manifest/sentinel set, durably record sorted unrestored paths, and exit 1. |
| All installed roots verified, before sentinel retirement | Sentinel still wins; restart restores the pre-run roots and exits 2 with rerun guidance. |
| Sentinel retired and directory-synced | Revalidate all three installed roots before any orphan cleanup or already-exact result; partial state never passes. |

## Responsibility boundaries

| Owner | Sole responsibility in this slice |
|---|---|
| `internal/adopt/baton_archive.go` | Compile-time archive embed and read-only bytes; no filesystem fallback. |
| `internal/baton/source.go`, `schemas/embed.go`, `manifest.go` | Ordinary mapping, dedicated ambiguity schema embed, and schema classification only. |
| `internal/baton/diff.go` | Public ordinary/archive parity and distinct upstream/adopting VERSION identity checks; `content.go` stays excluded. |
| `internal/baton/vendor.go`, `vendor_transaction.go` | One mapped-bytes + VERSION + archive repository plan, transaction, rollback, and restart recovery. |
| `internal/baton/installer_archive.go` | Deterministic archive construction/validation and native complete Codex/Claude tree generation. |
| `internal/baton/install_transaction.go` | Four-root topology preflight plus three-target staging, replacement, verification, whole-root rollback, durable sentinel, and recovery-only execution. |
| `cmd/sworn/baton.go`, `cmd/sworn/doctor.go` | Thin argument/path/result adapters and exact public exit mapping. |

No archive behavior belongs in `content.go`, ordinary schema `manifest.go`, or
`source.go`; no install transaction is hidden in the existing 1,316-line
doctor adapter.

## Rule-9 design decisions

All structural choices below mirror the durable `status.json` records.

- **Type-1 — canonical installer output and dual-install transaction.** Exact
  tagged scripts in isolated homes define independent oracle trees; native
  output must match both, and doctor may not leave the supported mirrors on
  different pins.
- **Type-1 — one embedded archive authority.** Repository parity, native
  generation, doctor checks, repair, and isolated proof consume one
  identity-checked archive; a live checkout is never runtime authority.
- **Type-1 — archive in the repository transaction.** Mapped bytes, VERSION,
  and archive share materialisation, snapshot, apply, rollback, and recovery;
  no standalone archive write can create mixed protocol state.
- **Type-1 — whole-root three-target rollback.** The complete pre-run
  `agents_home`, `codex_home`, and `claude_home` trees are the restoration
  outcome, preserving unrelated pre-existing content and preventing mixed
  installation success.
- **Type-1 — bounded responsibility placement.** Archive generation/validation,
  install transaction/recovery, binary embed, and public archive parity live in
  the focused owners ratified by the Coach; CLI adapters remain thin.
- **Type-1 — separate VERSION identities.** The fixed `5f1dd...` object proves
  only upstream root bytes `v0.15.1\n`. Each participating ref separately
  resolves the committed adopting-manifest blob, requires cross-ref blob
  equality, and parses tag/SHA/digest for comparison with marker and binary.
  Neither object substitutes for the other.
- **Type-1 — disjoint roots and pre-replacement recovery authority.** Physical
  resolution must prove all three target roots plus recovery are pairwise
  disjoint and free of aliases, nesting, symlinks, and special nodes before any
  mutation. Complete owner-only snapshots, manifest, and sentinel are durable
  before replacement; sentinel presence permits recovery only.
- **Type-2 — fixed installer modes.** Both exact script oracles explicitly set
  `umask 0022`; native output fixes directories to `0755` and files to `0644`,
  and a hostile inherited umask is a failing mutation if it can alter output.
- **Type-2 — path-only diagnostics.** Mismatch class, logical root, and sorted
  path are sufficient. Embedded payloads, credentials, request bodies,
  snapshots, and user-home bytes never enter logs or proof output.

## Planned files and acceptance trace

| File / surface | Planned change | AC |
|---|---|---|
| `internal/adopt/baton/VERSION`, `README.md`, `architecture.json`, `rules/07-adversarial-verification.md`, `rules/10-customer-journey-validation.md` | Exact mapped v0.15.1 bytes and separate parseable multi-line adopting-manifest result; upstream `v0.15.1\n` remains a distinct object. | AC-01, AC-02, AC-04 |
| `internal/adopt/baton/installer-input-v0.15.1.tar` | Exact validated 78-entry offline authority inside repository transaction. | AC-01, AC-03, AC-04 |
| `internal/adopt/baton_archive.go` | Sole compile-time archive embed and byte accessor. | AC-01, AC-03, AC-04 |
| `internal/prompt/implementer.md`, `planner.md`, `verifier.md` | Exact documented transformed tagged role prompts. | AC-01, AC-02 |
| `internal/prompt/baton/README.md`, `rules.md`, `track-mode.md`, `llm-checks/README.md`, `llm-checks/maintainability-review.md`, `llm-checks/spec-ambiguity.md` | Exact mapped/combined protocol and LLM-check bytes. | AC-01, AC-02, AC-03 |
| `internal/baton/schemas/board-v1.json`, `llm-check-report-v1.json`, `slice-status-v1.json`, `spec-v1.json`, `spec-ambiguity-report-v1.json` | Byte-exact schemas with ambiguity output independently embedded and classified. | AC-01, AC-02, AC-04 |
| `internal/baton/source.go`, `schemas/embed.go`, `manifest.go` | Add only the ambiguity-schema mapping/embed/classification. | AC-01, AC-02, AC-04 |
| `internal/baton/manifest_test.go`, `testdata/fixture/schemas/spec-ambiguity-report-v1.json` | Complete classification plus exact independent schema fixture. | AC-01, AC-02, AC-04 |
| `internal/baton/validate_schema_test.go` | Align only the two static positive slice-status literals with v0.15.1-required `start_commit` and `maintainability` members; no active-record migration or schema weakening. | AC-01 |
| `internal/baton/diff.go`, `diff_test.go` | Public mapped/archive parity, distinct upstream/adopting VERSION checks, deterministic drift, and operational failure coverage; `content.go` excluded. | AC-01, AC-02, AC-04 |
| `internal/baton/vendor.go`, `vendor_test.go`, `vendor_transaction.go`, `vendor_transaction_test.go` | Add validated archive candidate to the one repository plan and every transaction/recovery fault point. | AC-01, AC-02, AC-04 |
| `internal/baton/installer_archive.go`, `installer_archive_test.go` | Archive construction/safety, native fixed-mode complete-tree generation, fixed-0022 dual-script parity, and hostile-umask proof. | AC-01, AC-03, AC-04 |
| `internal/baton/install_transaction.go`, `install_transaction_test.go` | Four-root physical-disjointness preflight, three-root install, pre-replacement durable authority, whole-root rollback, and publish/replace/verify/retire/recovery fault matrix. | AC-03, AC-04 |
| `internal/baton/records_conformance_test.go` | Exact C-01 repository/archive/tree parity and layer-by-layer fail-closed mutations. | AC-01, AC-03, AC-04 |
| `internal/gate/llmcheck_test.go` | Add canonical `check` only to canned responses intended to satisfy v0.15; no requested-check matching or production parser change. | AC-05 |
| `internal/run/slice_test.go` | Supply explicit schema-valid maintainability facts while retaining the built RunSlice exact-schema assertion. | AC-05 |
| `internal/state/state.go`, `state_test.go`, `record_reconciliation_test.go` | Optional opaque supplied maintainability carrier and focused preservation/absence proof; fixtures supply valid start/maintainability facts with no defaults or lifecycle inference. | AC-05 |
| `cmd/sworn/baton.go`, `cmd/sworn/baton_test.go` | Thin public vendor/diff mapping and first binary reachability red, including exact 0/1/2 exits. | AC-02, AC-04 |
| `cmd/sworn/doctor.go`, `cmd/sworn/doctor_test.go` | Thin doctor parity/sync routing and built-binary three-root repair/recovery reachability. | AC-02, AC-03, AC-04 |

The table covers the exact 47 paths in `spec.json` and `status.json`; there is
no planned write to `internal/adopt/adopt.go` or `internal/baton/content.go`.

## First red, reachability, and proof

The first implementation red is
`cmd/sworn/baton_test.go:TestBatonDiffV015BinaryReachability`. It builds and
drives the registered `sworn baton diff` command through the binary, proving
exact parity exit 0, deterministic mapped/archive/schema/upstream-VERSION/
adopting-manifest drift exit 1, and malformed archive/mapping or operational
failure exit 2. This establishes the public C-01 gate before leaf archive or
transaction helpers exist.

The companion
`cmd/sworn/doctor_test.go:TestDoctorAndBatonDiffV015BinaryReachability` drives
ordinary `sworn doctor` and `sworn doctor --sync-baton` through the built binary
with isolated logical roots. It pairs exact state with exit 0, complete repair
and recovery-only completion with exit 2, and repository/archive/native-tree/
installed-tree/unsafe-topology/rollback-incomplete failures with exit 1. It also
proves fixed modes under hostile inherited umask and kill recovery at each
publish/replace/verify/retire boundary. The proof must show a live post-vendor
`vendor --check` exit 0 and `baton diff` exit 0.

Independent installer proofs set only temporary `HOME`, `AGENTS_HOME`,
`CODEX_HOME`, `CLAUDE_HOME`, and Sworn config paths. No required test or design
review writes the operator's real installations. Actual local mirror mutation
is an implementation action only after repository parity, fresh Captain
`PROCEED`, and Coach acknowledgement.

## Risks for fresh Captain review

- Confirm the archive candidate is validated before it joins the S01-derived
  plan and that recovery membership is derived from that complete plan, not a
  second hand-maintained write list.
- Confirm native Codex/Claude output derives its inventory from the archive but
  asserts the exact eight-name set, including `design-review.md`, while fixed
  `0755`/`0644` modes survive hostile inherited umask.
- Confirm `5f1dd...` is asserted only for upstream `v0.15.1\n`, while every
  participating ref's separate committed manifest blob and parsed pin agree.
- Confirm physical resolution rejects every alias/nesting/symlink/special-node/
  recovery-overlap topology before mutation, and that whole-root replacement
  preserves unrelated files.
- Confirm every publish, per-root replace/verify, rollback/recovery, and retire
  fault leaves either all three authoritative roots exact or the complete
  restart-authoritative owner-only sentinel set.
- Confirm `cmd/sworn` contains only argument/path/result adaptation and that
  path-only diagnostics cannot expose archive, credential, snapshot, or user
  bytes.
- Confirm exact normative JSON and both independent installer oracles are
  mandatory on every success path; neither prose transformation nor a single
  matching mirror can establish parity.

## Cycle-0 maintainability remediation — installer transaction typestate

Formal preflight finding `F-01` supersedes the earlier no-refactor assumption
for one bounded remediation in `internal/baton/install_transaction.go`. The
public `InstallOpts`, `CheckBatonInstall`, and `SyncBatonInstall` APIs and every
observable transaction/recovery behavior remain fixed.

Replace the reusable `preparedInstall` state bag with four transition-produced
private values:

- `installPreflight`: resolved disjoint roots, logical targets, captured path
  identities, and the fault seam; it owns no mutation state.
- `capturedInstall`: complete owner-only snapshots, canonical sorted manifest
  and bytes, operation/transaction identities, derived recovery paths, and the
  transferred owned-path ledger.
- `stagedInstall`: the captured authority plus complete verified desired stage
  trees and their stage identities.
- `publishedInstall`: validated durable recovery publication reconstructed from
  the staged transition or, on sentinel recovery, solely from sentinel, owner
  identity, manifest, and snapshots. Only replacement, restoration, unrestored
  updates, retirement, and durable control writes accept this value.

Targets become phase-specific wrappers rather than accumulating optional
`snapshot` and `stage` fields. Each transition returns a complete value or an
error; empty phase-required fields are invalid. The owned-path map is the sole
intentionally shared mutable resource ledger and transfers forward. Existing
algorithms, fault names/order, manifest-hash transaction identity, fsync/rename
boundaries, modes, topology/inode revalidation, error classes/rendering,
rollback, debris ownership, and recovery-only sentinel semantics remain
byte-for-byte where contractual. Remove the inert `needManifest` parameter from
`prepareInstall`; add no replacement option.

The remediation authors only `internal/baton/install_transaction.go`. Existing
tests remain the behavior oracle. After the tested semantic commit, the sealed
S02 bootstrap authorization must be replanned to the exact new review head
before the one permitted closure review can dispatch.
