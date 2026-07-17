# Design TL;DR — S20-v015-parity-portable-fixture

**Slice:** S20-v015-parity-portable-fixture · **Track:** T1-foundation ·
**Release:** 2026-07-15-baton-v0.16-conformance
**State:** design_review — no S20 product, vendor, archive, bundle, or local-installation bytes have been changed
**Covers:** N-01, N-02, N-10 · **Effort/complexity:** high / high (beast)

## User outcome

A protocol maintainer can prove exact Baton v0.15.1 parity from a clean checkout:
the built sworn binary, vendored protocol, schema manifest, offline installer
input, and complete isolated Codex and Claude mirrors agree without reading a
developer-specific Baton checkout.

## Authority and scope boundary

1. S19 is freshly verified at the live T1 head. Its immutable rollback proof
   establishes that the 45 non-release semantic paths from S02's frozen head
   2a17443d67d39cf681dba117a57673714a916d7f are back at S02's immutable
   start-tree boundary before this replacement begins. S20 re-delivers those
   semantics under its own lifecycle authority; it must not reuse S02 proof,
   claim, receipt, verifier result, or terminal deferral as evidence.
2. The prior implementation's Gate-3 portability defect was a built-binary
   reachability test that passed /home/brad/projects/baton to baton diff. S20
   replaces that ambient input with a committed, test-only Git bundle. The
   bundle is neither embedded nor a runtime source; production uses the one
   embedded installer archive only.
3. Every declared implementation surface belongs only to T1-foundation in the
   live touchpoint matrix. S20 must not touch S19 evidence, another track's
   code, a real HOME/.codex, HOME/.agents, or HOME/.claude tree, or any
   sibling/release branch.

## Proposed implementation

1. **Authenticate the clean-CI source before use.** Commit
   internal/baton/testdata/fixture/baton-v0.15.1.bundle as the normative
   2,505,826-byte bundle-v2 stream. The test first proves SHA-256
   cba3796ed382623f35abc568183e3a5a0d4a82335cebd4589989d0ae41b43ad5,
   Git blob 77e5b4cc7210a41ce8779bc352a1f487101fb80e, the ASCII header,
   complete history, annotated tag 3ba5f70435ff1ef3ea819def7b06c126fdb269d8,
   and peeled commit 3fb4d275ae8a151f6287e7b9279d71628b12eea0. Only then may it
   clone with --no-checkout beneath t.TempDir() and detach-check out
   v0.15.1^{commit}. The clone proves root VERSION blob
   5f1dd0af59642311ee04e018a0023562d4dde008 has exactly v0.15.1 plus LF,
   expected tree/archive identities, and clean status before becoming a binary
   input. Missing, corrupt, incomplete, wrongly tagged, wrongly peeled, dirty,
   or ambient sources fail before C-01 evidence is accepted.
2. **Materialise one exact parity candidate before repository mutation.** Add
   only the v0.13.1-to-v0.15.1 mapped delta plus spec-ambiguity-report-v1.
   The explicit source map, dedicated embed, schema fixture, and grade manifest
   enumerate their live set; normative JSON stays byte-identical and Markdown
   receives only documented command-reference transforms. Keep the upstream
   root VERSION object separate from Sworn's multi-line adopting VERSION
   manifest: the former proves source bytes, while each participating Sworn ref
   parses and compares the latter's tag, SHA, and digest.
3. **Make installer input a transaction member.** Build and validate the one
   78-entry installer-input-v0.15.1.tar from the pinned Git-archive operation
   before the first write. Pin SHA-256
   27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15,
   Git blob 39ae650dfe0282b0fa8bda14e1a01e7084077702, prefix, directory/file
   modes, paths, and source blobs. The validated archive joins mapped bytes and
   adopting VERSION in the existing byte-sorted vendor plan, snapshot, apply,
   verify, rollback, and recovery set. No direct archive write, alternate
   archive source, or separate recovery record is permitted.
4. **Generate complete managed trees from one embedded archive.**
   internal/adopt/baton_archive.go is the sole go:embed owner and returns a
   copied read-only byte view. internal/baton/installer_archive.go validates
   with Go stdlib archive/tar, rejecting traversal, absolute/non-canonical
   names, duplicates, links, devices, special nodes, invalid UTF-8,
   missing/extra inventory, and identity drift. From that source it generates
   Codex skills, the Codex Baton package, and Claude commands/Baton package.
   The exact command inventory is eight names including design-review.md.
   Native directories are explicitly 0755 and regular files 0644 regardless
   of inherited umask.
5. **Prove native generation with isolated script oracles.** Test-only
   extraction runs exact tagged install-codex.sh -y and install-claude.sh -y
   under an inner umask 0022 even when the outer process inherits 0077. Each
   oracle receives only temporary HOME, AGENTS_HOME, CODEX_HOME, CLAUDE_HOME,
   and Sworn config locations. Compare every managed path, byte, and mode to
   native output; compare Sworn-owned VERSION sentinels separately. No test or
   implementation step may modify a real Codex or Claude installation.
6. **Put install mutation and recovery below the doctor adapter.** Keep
   cmd/sworn/doctor.go responsible only for CLI path resolution, path-only
   rendering, and exact exit mapping. internal/baton/install_transaction.go
   owns physical root topology, staging, replacement, recovery, and fault
   seams. Before any snapshot or mutation, resolve agents_home, codex_home,
   claude_home, and recovery root; reject equal, nested, canonical-path alias,
   inode alias, symlink, special-file, invalid-path, and recovery-overlap
   cases. Snapshot whole roots, stage complete canonical trees and separate
   VERSION sentinels, then durably publish owner-only 0700/0600
   snapshot/manifest/sentinel authority before first replacement. Replace and
   verify all three roots as one transaction. A sentinel is the sole restart
   authority: a later doctor --sync-baton restores only, retains sorted
   unrestored paths on failure, and never combines recovery with a new install.
   Already exact exits 0; successful repair or recovery-only restoration exits
   2; failed repair, rollback, or recovery exits 1. Diagnostics expose logical
   paths/classes only, never payload, snapshot, or credential bytes.
7. **Carry required v0.15 schema facts without expanding lifecycle scope.**
   Add an optional json.RawMessage maintainability field to state.Status so a
   caller-supplied object survives Read/Write and validation. It stays absent
   when not supplied and gains no defaults, transition interpretation, record
   migration, or post-writer patching. Update only canned generic-check, state,
   and RunSlice fixtures with canonical check, start_commit, and
   maintainability facts. S03, S04, S05, and S17 retain typed/null semantics,
   requested-check matching, provenance, and lifecycle migration ownership.
8. **Start public reachability at the built binary.** The first red is
   cmd/sworn/doctor_test.go:TestDoctorAndBatonDiffV015BinaryReachability. It
   builds sworn, uses the authenticated temporary bundle clone as its only
   Baton source, drives sworn baton diff, sworn doctor, and sworn doctor
   --sync-baton against isolated roots, and pairs every verdict with 0/1/2
   exit behavior. A companion test rejects invalid bundles before clone.

## Planned surfaces and acceptance trace

| Surface | Planned change | AC |
|---|---|---|
| internal/adopt/baton mapped docs, VERSION, rules, and installer-input tar; internal/prompt mapped docs | Exact v0.15.1 mapped bytes, separate adopting manifest, and no unapproved transform. | AC-01, AC-02, AC-03 |
| internal/baton source.go, manifest.go, schemas/embed.go, schemas, and schema fixture | Add ambiguity-report mapping/embed/fixture/classification and exact schema bytes. | AC-01, AC-02, AC-05 |
| internal/baton vendor.go and vendor_transaction.go plus tests | Include the validated archive in every materialisation, rollback, and recovery member. | AC-01, AC-02, AC-04 |
| internal/adopt/baton_archive.go; internal/baton installer_archive.go and tests | Sole archive embed, identity/safety validation, deterministic native tree generation, and fixed-umask oracle parity. | AC-01, AC-03, AC-04 |
| internal/baton install_transaction.go and tests | Four-root preflight, whole-root snapshots/staging, durable sentinel, atomic repair, rollback, and recovery-only matrix. | AC-03, AC-04 |
| cmd/sworn baton.go, baton_test.go, doctor.go, doctor_test.go; internal/baton/diff.go | Thin 0/1/2 public adapter and built-binary proof with verified temporary bundle clone only. | AC-02, AC-04, AC-06, AC-07 |
| internal/baton records_conformance_test.go, manifest_test.go, validate_schema_test.go | Enumerate parity inputs and mutation-test mapping/schema/version/archive/tree drift without weakening validation. | AC-01, AC-02, AC-04, AC-05 |
| internal gate/run/state fixtures and state.go | Opaque supplied maintainability carrier and schema-valid canonical fixture facts only. | AC-05 |
| internal/baton/testdata/fixture/baton-v0.15.1.bundle | Exact committed test fixture used only to build the temporary source oracle. | AC-06, AC-07 |

## Test and proof strategy

- TestBatonV015ExactParity enumerates the live mapping, embed, manifest,
  source/adopting VERSION identities, schema map, fixture, and archive inventory;
  it rejects missing, extra, transformed, misclassified, or stale material.
- TestBatonV015GitBundleFixtureIdentity and the built-binary doctor tests verify
  bundle byte/blob/tag/commit/history/clean-clone identity, reject invalid
  bundles before clone, and contain no /home/brad/projects/baton dependency.
- TestBatonV015CodexAndClaudeMirrorParity compares native trees with both
  independent fixed-umask script outputs, including modes, frontmatter,
  argument prelude, path rewrites, all eight commands, and sentinels.
- TestBatonSyncRollbackAndRecovery fault-injects publication, replacement,
  verification, rollback, recovery, and retirement. It proves unsafe topology
  fails before mutation and only full-root restoration clears recovery authority.
- State/gate/run tests assert required fixture facts and lossless supplied
  maintainability round-trip while absence remains absent.
- Completion evidence will run the focused tests, go test ./... -count=1,
  go vet ./..., make build, a second vendor --check, and built sworn baton diff
  / doctor --sync-baton runs against only temporary roots and the verified clone.

## Collision, rollback, and non-delivery boundaries

- The T1 matrix authorizes only the declared surfaces. A need to edit another
  track's surface is a collision and stops implementation rather than expanding
  this slice.
- S20 does not alter S19 proof/status/journal or verified rollback authority and
  does not modify historical S02 lifecycle records. The frozen 45-path result
  is a semantic target, not permission to inherit old evidence.
- Repository archive parity is inseparable from mapped-byte/VERSION vendoring;
  an interrupted transaction restores the full candidate set before rerun.
- Install repair preserves complete pre-run user trees, including unrelated
  content. Crash or failed restoration retains durable authority and blocks
  ordinary install until exact recovery.
- The committed bundle is test-only. It cannot become an embed, production
  fallback, network substitute, or a reason to weaken exact tag/commit checks.

## Review pins for the fresh Captain

1. **[MECHANICAL]** Verify bundle byte/blob/header/history/tag/commit/root-VERSION/
   clean-clone checks happen before built-binary diff, with no ambient sibling
   path.
2. **[BOUNDARY]** Verify the test-only bundle never becomes runtime authority;
   the embedded installer archive remains the sole installer source.
3. **[MECHANICAL]** Verify upstream VERSION blob and Sworn's adopting manifest
   are independently proven on every relevant surface.
4. **[MECHANICAL]** Verify the 78-entry archive is validated before joining the
   one vendor transaction, with no direct write or separate recovery set.
5. **[MECHANICAL]** Verify both isolated script oracles and native generation
   derive the full eight-command inventory, including design-review.md, with
   0755/0644 output under hostile inherited umask.
6. **[MECHANICAL]** Verify root preflight rejects aliases/nesting/symlinks/
   special nodes/recovery overlap before mutation, and all roots roll back
   together under pre-published owner-only recovery authority.
7. **[BOUNDARY]** Verify maintainability is opaque/supplied only; S20 does not
   absorb S03/S04/S05/S17 lifecycle, typed-null, or migration work.
8. **[REACHABILITY]** Verify public tests use a built sworn binary, temporary
   homes, the authenticated bundle clone, and exact 0/1/2 exits.
