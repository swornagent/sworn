# Captain review — S02-v015-parity-and-installs
Date: 2026-07-16
Design commit: 956c1d61aff0124b7e74506836b1587e8910ff2f

## Pins

1. [escalate] §User outcome / AC-01 — The pinned VERSION blob has two incompatible live meanings
   What I observed: `5f1dd0af59642311ee04e018a0023562d4dde008` is the exact Git blob of upstream Baton v0.15.1 `VERSION`, whose complete bytes are `v0.15.1\n`. The live destination `internal/adopt/baton/VERSION` is instead Sworn's multi-line pin manifest; `internal/baton/version.go` requires its `baton-protocol:`, `upstream-sha:`, and `upstream-digest:` fields, and S01's `UpstreamPinReplacement` preserves that shape. Replacing the destination with the pinned upstream blob would break the binary, while preserving the manifest cannot give that destination the pinned blob OID. `protocol.json` and C-03 call the value `vendored_version_blob_oid`, but the revised design does not distinguish upstream source VERSION identity from the committed Sworn manifest identity.
   What to ask the implementer: Pause for Coach/Planner clarification of C-01/C-03. If `5f1dd...` means the upstream tag's VERSION blob, name and test it explicitly as upstream source identity while separately pinning/parsing Sworn's manifest. If it means the committed `internal/adopt/baton/VERSION` blob, re-plan the OID and deterministic bytes. Do not silently reinterpret the field or overwrite the manifest with `v0.15.1\n`.

2. [mechanical] §Proposed implementation.8 — Canonical managed-tree modes need a fixed oracle umask
   What I observed: the exact tagged installers use `mkdir`, `cp`, generated `cat` output, and `sed` without setting `umask`; the archived source modes are 0664/0775. Empty-home script output modes therefore vary with the invoking process even though AC-03 requires canonical byte/mode identity.
   What to ask the implementer: Run both isolated script oracles under one explicitly fixed safe umask, derive native file/directory modes from that canonical result, and mutation-test a hostile inherited umask so environment-dependent modes cannot pass.

3. [mechanical] §Proposed implementation.9-10 — The three-root transaction needs explicit alias and crash guards
   What I observed: `AGENTS_HOME`, `CODEX_HOME`, `CLAUDE_HOME`, and the Sworn recovery directory are caller/environment-derived paths. The design promises whole-root restoration and recovery-only restart behavior but does not explicitly reject equal/nested/aliased roots or recovery-root overlap, nor state that complete recovery authority is durable before the first root replacement. Without those guards, one logical replacement can overwrite another or process death can leave mixed roots without sole recovery authority.
   What to ask the implementer: Before mutation, physically resolve and prove all three targets and the recovery root are disjoint and non-symlinked, reject unsupported pre-existing symlink/special-file trees as the normative manifest requires, and durably publish the complete owner-only snapshot/manifest/sentinel authority before the first replacement; fault-test process death at every publish/replace/verify/retire boundary.

Pins: 3 total — 2 [mechanical], 0 [memory-cited], 1 [escalate]
Critical pins (if any): 1, 2, 3

## Summary

Pins: 3 total — 2 [mechanical], 0 [memory-cited], 1 [escalate]
Critical pins (if any): 1, 2, 3

## Smaller flags (not pins, worth one-line acknowledgement)

- All seven prior review pins are resolved: the design now declares `internal/adopt/baton_archive.go`, gives archive parity to `internal/baton/diff.go` with `content.go` excluded, places the archive inside mapped+VERSION transaction/recovery, assigns bounded archive/install owners, asserts all eight commands including `design-review.md`, records five Type-1 choices plus the Type-2 diagnostic default, and requires normative-byte plus dual-install fail-closed parity.
- The exact tag is `v0.15.1` at `3fb4d275ae8a151f6287e7b9279d71628b12eea0`; the archive operation independently reproduces 78 entries, SHA-256 `27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15`, and Git blob `39ae650dfe0282b0fa8bda14e1a01e7084077702`.
- S01's verified transaction implementation is the only relevant post-base ancestry and is explicitly consumed by the revised design. S03 shares `records_conformance_test.go` and `doctor.go` only as a later planned slice in the same serial track; no active sibling collision exists.
- The first-red and companion built-binary tests reach the registered `sworn baton diff`, `sworn doctor`, and `sworn doctor --sync-baton` surfaces with exact 0/1/2 behavior; no leaf-only reachability claim remains.
- This review used only committed release/slice artefacts, exact upstream-tag objects, and live repo state; no project or inherited memory was used as evidence.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR The revised ownership and parity design is implementation-ready once the Coach/Planner resolves the VERSION identity wording. 3 pins + 5 flags:

1. **Apply the ratified VERSION identity correction.** Implement the Coach/Planner's explicit distinction between upstream source VERSION identity and the committed Sworn pin manifest; do not reinterpret `vendored_version_blob_oid` locally or replace the manifest with bytes its parser cannot consume.
2. **Fix the installer oracle umask.** Run both exact script oracles under one declared safe umask, generate native modes to match, and prove hostile inherited umasks cannot change the canonical result.
3. **Harden the three-root transaction.** Reject equal, nested, aliased, symlinked, special-file, or recovery-overlapping roots before mutation and publish complete owner-only restart authority before the first replacement; kill-test every durable boundary.

Flags (not pins): (a) all seven prior pins are resolved; (b) archive SHA/blob/count reproduce exactly; (c) no active sibling collision exists; (d) public binary reachability is explicit; (e) no inherited memory or real installation mutation entered this review.

§2 decisions 1–6 acknowledged subject to the Planner-ratified VERSION wording. No open design question remains after that correction. Address pins 1–3 inline during implementation, then proceed to in_progress.

## Triage verdict

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: All prior design pins are resolved, but C-01/C-03 must distinguish the exact upstream VERSION blob from Sworn's differently shaped committed pin manifest before implementation can preserve both byte identity and binary parsing.
-->
