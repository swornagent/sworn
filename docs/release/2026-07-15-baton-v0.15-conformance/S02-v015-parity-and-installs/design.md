# Design TL;DR — S02-v015-parity-and-installs

**Slice:** `S02-v015-parity-and-installs` · **Track:** `T1-foundation` · **Release:** `2026-07-15-baton-v0.15-conformance`
**State:** `design_review` — no production code, tests, vendored bytes, archive, or Codex/Claude installation has been changed
**Covers:** `N-01`, `N-02`, `N-10` · **Effort/complexity:** high / high (`beast`) · **Boundary:** ratified 41-file bootstrap exception

## User outcome

The built `sworn` binary, the committed normative repository content, and both
supported user-level Baton installations agree on one authority: Baton
`v0.15.1`, upstream commit
`3fb4d275ae8a151f6287e7b9279d71628b12eea0`, source digest
`sha256:8f0839ea897374eb10d6db2a789939714727739621babef1117d74cbf4488d2f`,
and VERSION blob `5f1dd0af59642311ee04e018a0023562d4dde008`.
Any mapped byte, schema classification, archive
identity, native managed-tree output, installed tree, or Sworn sentinel mismatch
fails closed. Repair either installs both supported mirrors exactly or retains
owner-only recovery authority until all three logical roots have been restored
to their complete pre-run state.

## Readiness verdict

- S01 is `verified`; its vendor plan already materialises candidates before the
  first mutation and protects mapped files plus VERSION with one snapshot,
  apply, rollback, and restart-recovery transaction.
- The amended S02 spec has four concrete, traced EARS criteria. It explicitly
  owns the embed, public diff, expanded repository transaction, archive/native
  generation, three-root install transaction, thin CLI adapters, fixtures, and
  binary reachability tests required by C-01.
- The archive operation independently reproduces 78 entries, SHA-256
  `27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15`,
  and Git blob `39ae650dfe0282b0fa8bda14e1a01e7084077702` at the pinned commit.
- `status.json` remains at `design_review` with null `start_commit`, pending
  cycle 0, an empty maintainability ledger, and no adjudication. The Coach replan
  has resolved every prior Captain escalation; implementation still requires a
  fresh Captain `PROCEED` and Coach acknowledgement.

## Proposed implementation

1. **Make one complete archive candidate before repository mutation.** Resolve
   the exact clean `v0.15.1` checkout and C-01 tag/SHA/digest, then have
   `internal/baton/installer_archive.go` invoke the argv-equivalent pinned
   `git archive` operation. Validate the complete tar SHA-256, Git blob OID,
   78-entry path/mode/blob inventory, prefix, canonical names, and upstream
   object identities before exposing immutable candidate bytes.
2. **Expand S01's repository transaction, not its authority model.** Extend the
   vendor plan in `vendor.go` so mapped bytes, the generated VERSION candidate,
   and the validated archive candidate are all materialised before snapshots.
   Feed the byte-sorted combined candidate set to the existing
   `vendor_transaction.go` apply/verify/rollback machinery. Extend its recovery
   allow-set and tests to include exactly the archive destination; a direct tar
   write or separate recovery record is forbidden.
3. **Vendor only the mapped v0.15.1 delta.** Add
   `schemas/spec-ambiguity-report-v1.json` to the explicit mapping, dedicated
   schema embed, fixture, and grade classification. Preserve every normative
   JSON byte verbatim; apply only the already documented Markdown command/path
   transforms. Run the repaired upstream vendor transaction once, then require
   a second `vendor --check` and public `baton diff` to converge with zero drift.
4. **Give the archive one explicit binary owner.** Add
   `internal/adopt/baton_archive.go` with the sole `go:embed` directive for
   `baton/installer-input-v0.15.1.tar` and a read-only byte accessor. Production
   consumers use those embedded bytes only: no repository-path fallback, live
   Baton checkout, network fetch, or runtime shell is accepted.
5. **Keep public archive parity separate from ordinary mapped content.** Extend
   `internal/baton/diff.go` to consume the archive validator and compare the
   repository archive, compiled embed, pin, exact inventory/modes/blobs, and
   pinned upstream source. `content.go` remains unchanged and continues to own
   only ordinary mapped-file/rules materialisation. Deterministic drift returns
   public exit 1; malformed mapping/archive or operational failure returns 2.
6. **Generate both complete managed trees natively.** In
   `installer_archive.go`, safely parse the validated tar with Go stdlib
   `archive/tar`; reject traversal, absolute/non-canonical names, links, devices,
   special files, duplicates, invalid UTF-8, unexpected modes/types, or
   missing/extra inventory. Generate staged Claude and Codex trees using the
   exact tagged installer semantics, including Codex frontmatter reconstruction,
   argument-resolution prelude, and the documented Markdown/JSON path rewrites.
   The shipped path invokes no shell or installer script.
7. **Derive and assert the exact eight-command set.** Inventory comes from the
   validated archive and must equal, in unsigned-byte order:
   `design-review.md`, `implement-slice.md`, `mark-shipped.md`,
   `merge-release.md`, `merge-track.md`, `plan-release.md`,
   `replan-release.md`, and `verify-slice.md`. Claude receives all eight command
   files; Codex receives all eight corresponding `baton-*/SKILL.md` wrappers.
   Stale installer prose that says seven is not a second authority.
8. **Prove native output against two independent script oracles.** Tests extract
   the exact embedded archive into isolated proof directories, separately run
   its pinned `install-codex.sh -y` and `install-claude.sh -y` with empty
   temporary `HOME`, `AGENTS_HOME`, `CODEX_HOME`, and `CLAUDE_HOME`, and compare
   every managed path, mode, and byte to native output. Neither script shares
   native generation logic, and success on only one mirror is failure.
9. **Repair three roots as one install transaction.** Put staging, complete-tree
   manifests, whole-root replacement, post-install verification, rollback,
   `rollback-incomplete` persistence, and recovery-only routing in
   `internal/baton/install_transaction.go`. Stage complete pre-run-preserving
   target trees for logical `agents_home`, `codex_home`, and `claude_home`, add
   the canonical managed outputs and separate Sworn VERSION sentinels, verify
   all three, then replace under one transaction. `cmd/sworn/doctor.go` only
   resolves paths, renders path-only results, and maps outcomes to exits.
10. **Fail closed and preserve restart authority.** `doctor --sync-baton` exits
    0 only when all roots were already exact, 2 after a completely verified
    repair, and 1 for install/verification/rollback failure. An incomplete
    rollback preserves 0700 snapshots plus the exact manifest and fixed sentinel
    beneath `<sworn-config-dir>/recovery/baton-sync/`, reports unique
    unsigned-byte-sorted logical unrestored paths, and forces later sync runs to
    recovery-only execution. Exact recovery removes authority and exits 2 with
    rerun guidance; tamper or incomplete recovery remains exit 1.
11. **Freeze and prove the complete C-01 boundary.** Exact parity tests enumerate
    the live mapping/schema map rather than restating it, mutation-test every
    archive/mapping/schema/native/install layer, and pair verdicts with 0/1/2
    exits. After deterministic checks and proof are committed and clean, run the
    exact planning-authority Gate-8 Implementer preflight; the later fresh
    Verifier remains the only authoritative certification.

## Responsibility boundaries

| Owner | Sole responsibility in this slice |
|---|---|
| `internal/adopt/baton_archive.go` | Compile-time archive embed and read-only bytes; no filesystem fallback. |
| `internal/baton/source.go`, `schemas/embed.go`, `manifest.go` | Ordinary mapping, dedicated ambiguity schema embed, and schema classification only. |
| `internal/baton/diff.go` | Public ordinary plus archive parity; `content.go` stays excluded. |
| `internal/baton/vendor.go`, `vendor_transaction.go` | One mapped-bytes + VERSION + archive repository plan, transaction, rollback, and restart recovery. |
| `internal/baton/installer_archive.go` | Deterministic archive construction/validation and native complete Codex/Claude tree generation. |
| `internal/baton/install_transaction.go` | Three-root staging, replacement, verification, whole-root rollback, sentinel, and recovery-only execution. |
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
- **Type-2 — path-only diagnostics.** Mismatch class, logical root, and sorted
  path are sufficient. Embedded payloads, credentials, request bodies,
  snapshots, and user-home bytes never enter logs or proof output.

## Planned files and acceptance trace

| File / surface | Planned change | AC |
|---|---|---|
| `internal/adopt/baton/VERSION`, `README.md`, `architecture.json`, `rules/07-adversarial-verification.md`, `rules/10-customer-journey-validation.md` | Exact mapped v0.15.1 bytes and atomic pin result. | AC-01, AC-02, AC-04 |
| `internal/adopt/baton/installer-input-v0.15.1.tar` | Exact validated 78-entry offline authority inside repository transaction. | AC-01, AC-03, AC-04 |
| `internal/adopt/baton_archive.go` | Sole compile-time archive embed and byte accessor. | AC-01, AC-03, AC-04 |
| `internal/prompt/implementer.md`, `planner.md`, `verifier.md` | Exact documented transformed tagged role prompts. | AC-01, AC-02 |
| `internal/prompt/baton/README.md`, `rules.md`, `track-mode.md`, `llm-checks/README.md`, `llm-checks/maintainability-review.md`, `llm-checks/spec-ambiguity.md` | Exact mapped/combined protocol and LLM-check bytes. | AC-01, AC-02, AC-03 |
| `internal/baton/schemas/board-v1.json`, `llm-check-report-v1.json`, `slice-status-v1.json`, `spec-v1.json`, `spec-ambiguity-report-v1.json` | Byte-exact schemas with ambiguity output independently embedded and classified. | AC-01, AC-02, AC-04 |
| `internal/baton/source.go`, `schemas/embed.go`, `manifest.go` | Add only the ambiguity-schema mapping/embed/classification. | AC-01, AC-02, AC-04 |
| `internal/baton/manifest_test.go`, `testdata/fixture/schemas/spec-ambiguity-report-v1.json` | Complete classification plus exact independent schema fixture. | AC-01, AC-02, AC-04 |
| `internal/baton/diff.go`, `diff_test.go` | Public mapped/archive parity, deterministic drift, and operational failure coverage; `content.go` excluded. | AC-01, AC-02, AC-04 |
| `internal/baton/vendor.go`, `vendor_test.go`, `vendor_transaction.go`, `vendor_transaction_test.go` | Add validated archive candidate to the one repository plan and every transaction/recovery fault point. | AC-01, AC-02, AC-04 |
| `internal/baton/installer_archive.go`, `installer_archive_test.go` | Archive construction/safety plus native complete-tree generation and exact dual-script parity. | AC-01, AC-03, AC-04 |
| `internal/baton/install_transaction.go`, `install_transaction_test.go` | Three-root atomic install, whole-root rollback, sentinel integrity, and recovery-only fault matrix. | AC-03, AC-04 |
| `internal/baton/records_conformance_test.go` | Exact C-01 repository/archive/tree parity and layer-by-layer fail-closed mutations. | AC-01, AC-03, AC-04 |
| `cmd/sworn/baton.go`, `cmd/sworn/baton_test.go` | Thin public vendor/diff mapping and first binary reachability red, including exact 0/1/2 exits. | AC-02, AC-04 |
| `cmd/sworn/doctor.go`, `cmd/sworn/doctor_test.go` | Thin doctor parity/sync routing and built-binary three-root repair/recovery reachability. | AC-02, AC-03, AC-04 |

The table covers the exact 41 paths in `spec.json` and `status.json`; there is
no planned write to `internal/adopt/adopt.go` or `internal/baton/content.go`.

## First red, reachability, and proof

The first implementation red is
`cmd/sworn/baton_test.go:TestBatonDiffV015BinaryReachability`. It builds and
drives the registered `sworn baton diff` command through the binary, proving
exact parity exit 0, deterministic mapped/archive/schema/pin drift exit 1, and
malformed archive/mapping or operational failure exit 2. This establishes the
public C-01 gate before leaf archive or transaction helpers exist.

The companion
`cmd/sworn/doctor_test.go:TestDoctorAndBatonDiffV015BinaryReachability` drives
ordinary `sworn doctor` and `sworn doctor --sync-baton` through the built binary
with isolated logical roots. It pairs exact state with exit 0, complete repair
and recovery-only completion with exit 2, and repository/archive/native-tree/
installed-tree/rollback-incomplete failures with exit 1. The proof must also
show a live post-vendor `vendor --check` exit 0 and `baton diff` exit 0.

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
  asserts the exact eight-name set, including `design-review.md`.
- Confirm whole-root replacement preserves unrelated files and that every
  install, verify, rollback, and recovery fault leaves either all three roots
  exact or one restart-authoritative sentinel.
- Confirm `cmd/sworn` contains only argument/path/result adaptation and that
  path-only diagnostics cannot expose archive, credential, snapshot, or user
  bytes.
- Confirm exact normative JSON and both independent installer oracles are
  mandatory on every success path; neither prose transformation nor a single
  matching mirror can establish parity.
