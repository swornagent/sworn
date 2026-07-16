# Captain review — S02-v015-parity-and-installs
Date: 2026-07-16
Design commit: 38b0dd4e7ef6f21389f02089ae4344f2d34e40f5

## Pins

1. [escalate] §Review pins.1 — The offline archive has no declared binary-embed owner
   What I observed: `internal/adopt/adopt.go` owns the only `embed.FS` that exposes `internal/adopt/baton/*`, and its directive does not include `installer-input-v0.15.1.tar`. `cmd/sworn` cannot embed across `..`, so the shipped binary needs either this file or a new file in `internal/adopt`; neither is in the ratified 28-file boundary.
   What to ask the implementer: After Coach/planner re-planning, use the selected explicit embed owner and expose only validated archive bytes through `internal/adopt`; do not fall back to the repository filesystem at runtime.

2. [escalate] §Review pins.2 — Public archive parity requires an undeclared `baton diff` owner
   What I observed: C-01 and normative clarification §2 require repository parity to verify the archive SHA-256, Git blob, safe inventory, modes, and entry blobs. The live `Diff` implementation in `internal/baton/diff.go` only iterates `batonFileMappings`; `internal/baton/content.go` only materialises ordinary mapped files and the combined-rules sentinel. The archive is deliberately not an ordinary mapping, so `source.go` or `manifest.go` alone cannot make the public check fail closed.
   What to ask the implementer: Re-plan the smallest explicit archive-parity owner including `internal/baton/diff.go`. Do not add `content.go` by reflex: own it only if the Coach-selected design deliberately places archive materialisation there; otherwise keep ordinary mapped content and archive validation separate.

3. [escalate] §Proposed approach.4 — The archive is outside the repository vendor transaction and recovery authority
   What I observed: S01's live vendor plan and restart recovery accept exactly `batonFileMappings + VERSION`. The design constructs the tar before running that transaction but names no mechanism that makes the tar a candidate, snapshot, rollback member, or recovery member. A direct archive write can therefore survive a failed mapped-byte/VERSION transaction, or the mapped-byte/VERSION transaction can succeed while archive publication fails.
   What to ask the implementer: The Coach must choose and the Planner must record either (a) expansion of the repository transaction/recovery boundary to include the fully validated archive, with the necessary `vendor.go` and `vendor_transaction.go` ownership, or (b) an explicit amended split-transaction contract with exact failure semantics. Do not implement an unrecorded standalone tar write.

4. [escalate] §Review pins.3 — Archive generation and three-root recovery placement is an unrecorded architectural choice
   What I observed: `cmd/sworn/doctor.go` is already 1,316 lines, while the design adds hostile-tar validation, native Codex/Claude generation, complete-tree manifests, a three-root transaction, rollback-incomplete persistence, and recovery-only routing. The ratified file ceiling provides no semantically named helper owner, and `manifest.go` is currently the schema-grade manifest rather than a filesystem transaction package.
   What to ask the implementer: The Coach must choose between bounded new internal owners (and a re-planned file ceiling) or explicit acceptance of placement in the existing large files. Record the choice, options, and rationale as Type-1 before code.

5. [mechanical] §Proposed approach.7 — The exact tag installs eight commands, not seven
   What I observed: The design says native Claude generation will "copy the seven command files", but Baton v0.15.1 contains eight files under `commands/`, including `design-review.md`; both tagged installers glob `commands/*.md`. Encoding seven would make both canonical trees incomplete and violate AC-03/AC-04.
   What to ask the implementer: Enumerate the complete tagged command set, assert all eight outputs including design-review, and derive both native trees from the exact script-visible inventory rather than stale installer prose or a hard-coded seven-file list.

6. [mechanical] §Design choices — Rule 9 records are incomplete and two structural choices are misclassified Type-2
   What I observed: `status.json` records one combined Type-1 decision, while design.md separately labels staged whole-root rollback and one embedded source as Type-2 and omits all Type-2 defaults from `design_decisions`. Both of those choices shape the cross-home transaction and protocol authority and are architecturally significant; the unresolved helper-placement choice is also Type-1-equivalent.
   What to ask the implementer: In the re-plan/acknowledgement, fold the structural choices into explicit Type-1 records with the applicable Coach decision, record the remaining Type-2 path-only diagnostic default, and add the selected responsibility-placement decision before transitioning to code.

7. [memory-cited] §Design choices.1 — Exact normative bytes and dual-install proof preserve the prior parity lesson
   What I observed: The design's byte-exact JSON policy and independent Codex/Claude checks align with the previous Sworn Baton upgrade, where a stale normative schema escaped until vendor and diff shared exact schema-aware policy and both local mirrors were checked.
   What to ask the implementer: Preserve that contract across the new schema, archive, public diff, doctor, and isolated installer proofs; no prose transform or success on only one mirror may count as parity.
   Citation (if [memory-cited]): [[Baton v0.13.1 prerequisite upgrade and parity verification]]

Pins: 7 total — 2 [mechanical], 1 [memory-cited], 4 [escalate]
Critical pins (if any): 1, 2, 3, 5

## Summary

Pins: 7 total — 2 [mechanical], 1 [memory-cited], 4 [escalate]
Critical pins (if any): 1, 2, 3, 5

## Smaller flags (not pins, worth one-line acknowledgement)

- The exact archive command independently reproduces 78 entries, SHA-256 `27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15`, and Git blob `39ae650dfe0282b0fa8bda14e1a01e7084077702` at the pinned commit.
- The live pre-v0.15 `sworn baton diff` reports exactly the 17 mapped destination drifts named by the design; no additional ordinary mapped file is missing from its planned set.
- S03 later shares `internal/baton/records_conformance_test.go` and `cmd/sworn/doctor.go`, but it is still planned in the same serial track, so there is no active sibling collision.
- No planned S02 file has cross-release ancestry on the current track beyond the release base.
- The design correctly keeps real Codex/Claude installations untouched until repository parity and approval; this review performed no vendoring or installation mutation.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR The conformance direction is sound, but implementation proceeds only from the Coach-selected, planner-recorded ownership and transaction boundary. 7 pins + 5 flags:

1. **Use the re-planned embed owner.** Consume `installer-input-v0.15.1.tar` only through the explicit `internal/adopt` embed boundary; no repository-filesystem runtime fallback.
2. **Make public diff own archive parity.** Use the re-planned `internal/baton/diff.go` boundary to fail closed on archive identity, inventory, mode, and blob drift; keep ordinary mapped-content materialisation separate unless the recorded design explicitly says otherwise.
3. **Keep the archive inside the recorded repository transaction contract.** Implement the Coach-selected, planner-recorded archive publication and rollback semantics; no standalone unprotected tar write.
4. **Use the recorded responsibility placement.** Put archive generation and three-root recovery in the bounded owners selected during re-planning, and preserve the Type-1 rationale.
5. **Generate all eight commands.** Derive the complete command/skill set from the exact tagged inventory and include `design-review.md` in Claude and Codex output.
6. **Repair Rule 9 records.** Record every structural choice as Type-1 with its Coach authority, record the path-only diagnostic Type-2 default, and include the selected helper-placement decision.
7. **Preserve exact-byte parity.** Keep normative JSON verbatim and require both Codex and Claude mirrors plus every public archive surface to agree before success.

Flags (not pins): (a) archive SHA/blob/count independently match; (b) the live ordinary mapping has exactly 17 expected drifts; (c) S03 shared files remain serial and planned; (d) no S02 planned-file ancestry collision exists; (e) no real install or vendor mutation occurred during review.

The Coach-selected re-plan and §2 Type-1/Type-2 records are acknowledged. The six review questions are resolved by that durable boundary plus pins 5–7. Address pins 1–7 inline during implementation, then proceed to in_progress.

## Triage verdict

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: The exact archive lacks declared embed, public-diff, and repository-rollback owners, and choosing the required file-boundary expansion and responsibility placement exceeds Captain authority.
-->
