# Captain review — S02-v015-parity-and-installs
Date: 2026-07-16
Design commit: e571293d0da97d714d6bb6c0c2be7b6efcb0b916

## Pins

No pins.

Pins: 0 total — 0 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): none

## Summary

Pins: 0 total — 0 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): none

## Smaller flags (not pins, worth one-line acknowledgement)

- The original ownership pins remain resolved: `internal/adopt/baton_archive.go` is the sole binary archive owner, `internal/baton/diff.go` owns public archive parity, the archive joins mapped bytes and VERSION in S01's repository transaction, archive/install behavior has bounded internal owners, and both CLI files stay thin.
- The exact upstream object checks agree with the design: tag `v0.15.1` resolves to `3fb4d275ae8a151f6287e7b9279d71628b12eea0`; blob `5f1dd0af59642311ee04e018a0023562d4dde008` contains only `v0.15.1\n`; the installer archive operation reproduces 78 entries, SHA-256 `27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15`, and blob `39ae650dfe0282b0fa8bda14e1a01e7084077702`; the tag has all eight commands including `design-review.md`.
- The second replan now gives the two VERSION identities non-overlapping meanings: the fixed upstream root blob is source evidence only, while every participating ref separately resolves and parses Sworn's committed multi-line adopting manifest.
- Canonical installer modes are deterministic: both independent script oracles set `umask 0022`, native generation explicitly fixes directories to `0755` and regular files to `0644`, and hostile inherited umask is a required negative fixture.
- Install safety is fail-closed before mutation: all four roots are physically resolved and pairwise checked for equality, nesting, aliases, symlinks, special nodes, and recovery overlap; complete `0700`/`0600` snapshot, manifest, and sentinel authority is durable before the first replacement.
- The recovery-only matrix covers incomplete publication, every root replacement and verification, rollback/recovery failure, and authority retirement. Sentinel presence never permits a new install; success is either exact installed state after durable retirement or exact pre-run restoration with rerun guidance.
- S01 is verified and its transaction seams are the only relevant post-base production ancestry. Later S03/S05 planned overlap is serial within T1, and no active sibling collision exists.
- The first red and companion built-binary tests reach registered `sworn baton diff`, `sworn doctor`, and `sworn doctor --sync-baton` paths with exact 0/1/2 behavior; deterministic gates `designfit`, trace, AC shape, touchpoints, status, spec quality, and requirements validation pass.
- This review used committed release artefacts, exact upstream-tag objects, and live repository state only. Project memory and the memory-reading LLM design check were intentionally excluded by the fresh-review evidence boundary.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR The twice-replanned C-01 design is implementation-ready. 0 pins + 9 flags.

Flags (not pins): (a) the archive embed, public parity, repository transaction, bounded helper, thin-adapter, eight-command, and binary-reachability owners are explicit; (b) the upstream commit, VERSION blob, 78-entry archive SHA/blob, and command inventory reproduce exactly; (c) upstream VERSION and Sworn adopting-manifest identities remain distinct; (d) script and native modes are fixed under `umask 0022` with hostile-umask coverage; (e) four-root topology fails closed before mutation; (f) complete owner-only recovery authority is durable before replacement; (g) publish/replace/verify/rollback/recovery/retire crash states are recovery-only; (h) no active sibling collision exists; (i) no project memory or real installation state entered this review.

All nine Rule-9 decisions are acknowledged. No open design question remains. Proceed to `in_progress` and implement the design exactly as written; the fresh Verifier remains the certification backstop.

## 2026-07-16 — Fresh Captain review of repository-gate carrier replan

**Verdict:** `PROCEED` — 0 pins. Constitutional slice-wide review; no Coach
escalation is required for this bounded delta.

- Add only an optional opaque carrier, preferably `json.RawMessage` with
  `omitempty`, and prove supplied-object preservation plus absent-field
  behavior. Do not type or interpret maintainability.
- Update only canned generic responses with their correct canonical `check`
  identities. Production parsing and requested/emitted equality remain S04.
- Supply valid 40-hex `start_commit` values and complete maintainability objects
  in the RunSlice/state fixtures. Do not change `StartCommit`, transitions,
  defaults, migrations, validation authority, or post-write JSON.
- Do not rewire `state.Write` validation/atomicity; S03 retains typed/null/
  additive semantics and exact-schema atomic-writer authority.
- The reproduced ten failures match the amended five-path design exactly. The
  2,645-line install transaction owner is orthogonal and needs no change for
  this delta.
- Preserve immutable `start_commit` and the existing semantic implementation;
  the Implementer must reconfirm the high/high `beast` rating before resuming.

Critical pins: none.

## Triage verdict

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: yes
REASON: The design now separates both VERSION identities and fully specifies deterministic installer modes, disjoint-root preflight, pre-replacement owner-only recovery authority, complete crash recovery, bounded ownership, and public fail-closed reachability.
-->

## 2026-07-16 — Fresh Captain review of cycle-0 maintainability remediation

**Verdict:** `PROCEED` — five mechanical pins, no escalation.

1. Replace `preparedInstall` with complete transition-produced
   `installPreflight`, `capturedInstall`, `stagedInstall`, and
   `publishedInstall` private values. Do not reintroduce optional phase fields
   on a reusable bag or on `installTarget`.
2. Fresh recovery reconstructs published authority only from the validated
   sentinel, owner identity, manifest, and snapshots. It must not fabricate
   captured/staged state or permit new installation work while a sentinel
   exists.
3. Preserve fault point names/order, transaction identity, fsync/rename
   boundaries, modes, topology/inode checks, rollback, debris ownership,
   path-only diagnostics, and `InstallError` behavior.
4. Keep `InstallOpts`, `CheckBatonInstall`, and `SyncBatonInstall` unchanged;
   remove the inert `prepareInstall` `needManifest` argument.
5. Author only `internal/baton/install_transaction.go`; existing tests are the
   public and crash-recovery behavior oracle.

The owned-path map is the sole intentionally shared mutable ledger and transfers
forward. Phase-specific helpers accept only the invariants they require.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: yes
REASON: The formal in-scope finding is satisfiable by a mechanical four-phase private typestate extraction in the single authorized file without changing public APIs or transaction semantics.
-->
