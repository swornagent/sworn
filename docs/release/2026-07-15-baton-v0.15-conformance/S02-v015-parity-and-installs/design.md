# Design TL;DR — S02-v015-parity-and-installs

**Slice:** `S02-v015-parity-and-installs` · **Track:** `T1-foundation` · **Release:** `2026-07-15-baton-v0.15-conformance`
**State:** `design_review` — no vendoring, pin update, production/test edit, or local installation mutation performed
**Covers:** `N-01`, `N-02`, `N-10` · **Effort/complexity:** high / low (`grind`)

## User outcome

The built `sworn` binary, the committed repository embed, and both supported
user-level Baton installations will agree on one exact authority: Baton
`v0.15.1`, upstream commit
`3fb4d275ae8a151f6287e7b9279d71628b12eea0`, source digest
`sha256:8f0839ea897374eb10d6db2a789939714727739621babef1117d74cbf4488d2f`,
and VERSION blob `5f1dd0af59642311ee04e018a0023562d4dde008`.
Any mapped byte, schema classification, offline installer input, generated
Codex/Claude tree, or local sentinel mismatch fails closed. A requested repair
either leaves all three target roots exact or retains restart-authoritative,
owner-only recovery material until their complete pre-run state is restored.

## Readiness verdict

- All four acceptance criteria are singular EARS conditions with concrete
  paths, test names, hashes, modes, exit codes, and recovery outcomes. The live
  `lint ac`, `lint trace`, `reqvalidate`, `designfit`, and `specquality` gates
  pass for the release.
- The exact upstream tag resolves to the pinned commit. The specified Git
  archive operation independently reproduces 78 entries, SHA-256
  `27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15`,
  and Git blob `39ae650dfe0282b0fa8bda14e1a01e7084077702`.
- S01 is verified and its atomic mapped-plus-VERSION transaction is reachable
  from the clean T1 head. S02 remains the pristine cycle-0 maintainability
  record with null `start_commit`.
- **Boundary readiness is conditional.** Two required implementation owners are
  absent from the ratified T1 touchpoints. The Captain must pin them and route
  any boundary expansion through the Coach/planner before implementation; the
  Implementer must not hide them in unrelated declared files.

## Proposed approach after Captain/Coach clearance

1. **Freeze one upstream authority.** Resolve the clean Baton checkout at tag
   `v0.15.1`, require its commit and source digest to equal C-01, and construct
   the exact archive using the normative `git archive --format=tar
   --prefix=baton-v0.15.1/` path set. Validate the tar byte hash, Git blob OID,
   78-entry inventory, modes, and per-entry blobs before it can become an
   embedded installer source.
2. **Extend the explicit repository mapping before the bump.** Add
   `schemas/spec-ambiguity-report-v1.json` to the one source/destination map,
   add a dedicated `go:embed` variable and independent `SchemaMap` key, copy
   the exact schema fixture, and add an explicit grade-manifest row. The
   classification must name its actual enforcement boundary rather than
   treating it as `llm-check-report-v1` or allowing an unclassified schema.
3. **Drive the first red through public reachability.** Start with
   `cmd/sworn/doctor_test.go:TestDoctorAndBatonDiffV015BinaryReachability` so the
   registered `sworn doctor`, `sworn doctor --sync-baton`, and `sworn baton
   diff` surfaces expose C-01 and their 0/1/2 exits. Leaf tests then isolate
   schema inventory, archive validation, installer transformation, and
   rollback/recovery faults.
4. **Run S01's vendor transaction exactly once after its inputs are complete.**
   Use upstream write mode against the pinned tag so the 17 currently drifted
   mapped destinations, the new schema destination, and the complete VERSION
   replacement are materialised before the first mutation and committed by the
   existing byte-sorted rollback-protected transaction. Do not hand-edit the
   pin or copy mapped Markdown/JSON around the transaction.
5. **Prove immediate repository convergence.** Re-run upstream `vendor
   --check`, then `sworn baton diff`, and require both to exit 0 with no drift.
   The parity test enumerates the live mapping rather than restating it, asserts
   the VERSION blob and every normative JSON byte, rejects missing/extra or
   transformed schemas, and separately verifies the embedded archive's exact
   hash/OID/inventory against the pinned upstream tree.
6. **Treat the embedded tar as hostile until fully validated.** The shipped
   stdlib reader uses `archive/tar`, rejects absolute paths, traversal,
   non-canonical names, links, devices, duplicate or missing entries, invalid
   UTF-8, unexpected types/modes, and any hash/inventory mismatch. It never
   extracts directly into a live home; validated regular-file bytes first form
   an immutable in-memory/staged source tree.
7. **Generate canonical Claude output natively.** Reproduce the exact pinned
   `install-claude.sh -y` semantics in an empty staged home: copy the seven
   command files, the complete `baton/` tree, and all schema files into
   `baton/schemas/`, preserving the canonical managed paths and file modes.
   Independent execution of the exact tagged script in an isolated proof home
   is the oracle; the shipped binary never invokes a shell.
8. **Generate canonical Codex output natively.** Reproduce the exact pinned
   `install-codex.sh -y` pipeline: copy `baton/`, rewrite only the documented
   Claude-to-Codex Baton paths in Markdown/JSON, and create one
   `baton-<command>/SKILL.md` per command by extracting `description`, stripping
   source frontmatter, applying the path rewrite, and injecting the exact
   argument-resolution prelude and skill frontmatter. Independent exact-script
   output in empty `CODEX_HOME`/`AGENTS_HOME` must byte/mode-match the native
   staged trees before any local install.
9. **Make ordinary doctor fail closed on every C-01 layer.** Validate the
   embedded pin, mapped repository parity, schema map/classification, tar
   identity/safety, native-vs-script canonical output, and complete Codex and
   Claude managed trees plus Sworn-owned VERSION sentinels. Any embedded or
   installed mismatch is `ERROR` and exit 1; diagnostics name only classes and
   sorted paths, never protocol payloads, credentials, request bodies, or
   snapshot bytes.
10. **Repair three logical roots as one transaction.** For
    `doctor --sync-baton`, resolve the isolated/default `agents_home`,
    `codex_home`, and `claude_home`, snapshot their complete pre-run state,
    overlay only Baton's managed outputs and sentinels in staging, verify all
    three staged roots, then replace them under one rollback boundary. Exit 0
    when already exact, 2 only after successful repair, and 1 after failed
    repair with verified complete restoration.
11. **Make incomplete rollback restart-authoritative.** If restoration is
    incomplete, preserve owner-only snapshots and the exact binary manifest
    beneath `<sworn-config-dir>/recovery/baton-sync/<transaction-sha256>/`,
    atomically publish `rollback-incomplete.json`, sort logical unrestored paths
    bytewise, and make later sync invocations recovery-only. Tamper, traversal,
    foreign material, symlink, mode drift, or digest mismatch retains the
    sentinel and exits 1; exact recovery removes material and exits 2 with
    re-run guidance, never combining recovery with a new install.
12. **Apply the planning-authority Gate-8 adapter after stable proof.** Once
    deterministic checks, proof, and public reachability are green and the
    semantic checkpoint is committed cleanly, construct the exact v0.15.1
    first-parent non-merge scope and `baton-maintainability-v1` fingerprint from
    immutable `start_commit`; invoke the untouched tagged prompt at temperature
    0; validate and blob-pin the Implementer cycle-0 preflight report; and freeze
    its `review_scope.head` as `maintainability.implementation_head`. A fresh
    Verifier owns the distinct authoritative run. The installed legacy generic
    maintainability check is not evidence for this planning-authority release.

## Design choices

- **Type-1 — exact installer output is authority:** the tagged shell scripts run
  only in isolated proof homes and define canonical bytes/modes; production uses
  a stdlib-native reproduction over the validated embedded tar.
- **Type-1 — dual-install atomicity:** Codex and Claude plus their sentinels form
  one repair outcome. A command may not report success while supported mirrors
  advertise different pins.
- **Type-2 — staged whole-root rollback:** construct and verify all target trees
  before mutation, retain unrelated pre-existing home content byte-for-byte,
  and verify full restoration rather than trusting filesystem return values.
- **Type-2 — one embedded source:** repository mapping, native generation,
  doctor checks, install repair, and isolated parity all consume the same
  identity-checked tar; no live Baton checkout is a runtime dependency.
- **Type-2 — path-only diagnostics:** expose mismatch class, logical target, and
  sorted path only. Owner-only recovery snapshots may contain user bytes and
  therefore never enter logs or proof payloads.
- **Boundary — later slices interpret v0.15 semantics:** S02 embeds and proves
  exact contracts; S03–S05 and S17 own record interpretation/migration, and
  S06–S13 own generalized maintainability/lifecycle operations and cutover.

## Planned files and AC trace

| File / surface | Planned change | AC |
|---|---|---|
| `internal/adopt/baton/VERSION` | Atomic transaction result for exact tag/SHA/digest/date and pinned VERSION blob. | AC-01, AC-04 |
| `internal/adopt/baton/{README.md,architecture.json,rules/07-adversarial-verification.md,rules/10-customer-journey-validation.md}` | Exact mapped v0.15.1 content changes only. | AC-01, AC-02 |
| `internal/prompt/{implementer.md,planner.md,verifier.md}` | Exact transformed tagged role prompts. | AC-01, AC-02 |
| `internal/prompt/baton/{README.md,rules.md,track-mode.md,llm-checks/README.md,llm-checks/maintainability-review.md,llm-checks/spec-ambiguity.md}` | Exact mapped/combined v0.15.1 protocol and LLM-check bytes. | AC-01, AC-02, AC-03 |
| `internal/baton/schemas/{board-v1.json,llm-check-report-v1.json,slice-status-v1.json,spec-v1.json,spec-ambiguity-report-v1.json}` | Byte-exact schemas, with ambiguity output distinct and addressable. | AC-01, AC-02, AC-04 |
| `internal/baton/source.go` | Add the ambiguity-schema source mapping and expose exact mapped inventory to parity checks. | AC-01, AC-02, AC-04 |
| `internal/baton/schemas/embed.go` | Embed and key the dedicated ambiguity schema. | AC-01, AC-02, AC-04 |
| `internal/baton/manifest.go` | Classify the new schema and expose deterministic archive/managed-tree identity data without duplicating payload bytes. | AC-01, AC-02, AC-04 |
| `internal/baton/manifest_test.go` | `TestV015SchemaManifestComplete`, including missing/extra/misclassified mutation cases. | AC-02, AC-04 |
| `internal/baton/testdata/fixture/schemas/spec-ambiguity-report-v1.json` | Exact independent fixture for the dedicated schema contract. | AC-01, AC-02 |
| `internal/adopt/baton/installer-input-v0.15.1.tar` | Exact 78-entry offline installer source with pinned tar and Git-blob identities. | AC-01, AC-03, AC-04 |
| `internal/baton/records_conformance_test.go` | Exact repository/archive parity, independent exact-script vs native output parity, and layer-by-layer fail-closed mutation matrix. | AC-01, AC-03, AC-04 |
| `cmd/sworn/doctor.go` | C-01 checks, native canonical generators, dual-install transaction, rollback-incomplete sentinel, and recovery-only routing. | AC-02, AC-03, AC-04 |
| `cmd/sworn/doctor_test.go` | Public command/binary reachability and every install/verify/rollback/recovery fault boundary with paired exits. | AC-02, AC-03, AC-04 |

## Reachability and proof plan

The first red is
`cmd/sworn/doctor_test.go:TestDoctorAndBatonDiffV015BinaryReachability`, driving
the registered CLI and built binary. It pairs verdict and exit for exact doctor
state, repository/archive/schema drift, already-exact sync (0), successful
dual-install repair/recovery-only completion (2), and failed repair or
rollback-incomplete recovery (1). `sworn baton diff` must remain read-only with
exit 0 only for exact parity, 1 for deterministic drift, and 2 for malformed
mapping/operational failure.

The isolated installer proof sets temporary `HOME`, `CODEX_HOME`, `AGENTS_HOME`,
`CLAUDE_HOME`, and Sworn config paths. It executes the extracted exact scripts
only there, compares complete path/mode/byte manifests against independent
native staged output, mutates each transformation/archive/local layer, and
fault-injects every install, verification, rollback, and recovery step. No proof
step reads or writes the operator's real Codex/Claude installations. Actual
local mirror mutation is a final post-parity implementation action only after
Captain `PROCEED` and Coach acknowledgement.

## Review pins for the fresh Captain

1. **[ESCALATE / OWNERSHIP] The tar is not embedded by any declared file.**
   `internal/adopt/adopt.go` owns `batonFS` and currently embeds only README,
   VERSION, rules, and architecture. A shipped binary cannot read
   `internal/adopt/baton/installer-input-v0.15.1.tar` without changing that
   directive (or adding another explicitly owned embed surface), but no T1 slice
   claims `internal/adopt/adopt.go`. Require a planner touchpoint correction;
   do not permit a repository filesystem fallback.
2. **[ESCALATE / OWNERSHIP] Archive parity has no declared `baton diff` owner.**
   `internal/baton/diff.go` directly iterates `batonFileMappings` and
   `internal/baton/content.go` can materialise only ordinary mapped files or the
   combined rules sentinel. The generated installer tar cannot be made an
   ordinary source mapping. If C-01/AC-04 require `sworn baton diff` to detect
   archive identity/content drift, the planner must add the smallest explicit
   diff/materialisation owner(s); hiding archive logic in `source.go` or
   `manifest.go` would violate maintainability and the declared responsibility
   boundary.
3. **[DESIGN] Keep native generation separate from transaction mechanics.** The
   current 28-file ceiling leaves no semantically named production helper for
   archive extraction, managed-tree generation, or three-root rollback. Confirm
   whether the Coach intends those responsibilities to live in the already-large
   `doctor.go`/generic manifest files or will ratify bounded new internal files.
   Gate-8 is likely to reject an unrelated god-file workaround.
4. **[MECHANICAL] Exact archive identity is independently reproduced:** 78
   entries, SHA-256 `27d5021…a15`, Git blob `39ae650…702`, with safe extraction
   and per-path/mode/blob comparison against commit `3fb4d275…ea0`.
5. **[SECURITY] Local repair is a three-root transaction:** validate/stage before
   mutation, preserve unrelated user content, never log payload/credential or
   snapshot bytes, and keep owner-only restart authority on incomplete rollback.
6. **[BOUNDARY] Gate-8 remains exact and manual under planning authority:** use
   the tagged lifecycle/scope/prompt/report contract after a clean stable
   checkpoint; do not use the installed legacy generic check and do not mutate
   real installations before design approval and repository parity.
