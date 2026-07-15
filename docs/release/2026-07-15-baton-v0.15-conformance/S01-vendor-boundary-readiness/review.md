# Captain review — S01-vendor-boundary-readiness
Date: 2026-07-16
Design commit: 7d95fb0e007c6f82fd24d8230b6cfd4158841c36

## Pins

1. [escalate] §6.1 — The public exit and upstream-pin transaction boundary is outside S01's ratified scope
   What I observed: AC-03/AC-04 require `sworn baton vendor` to return drift as exit 1 and invalid, operational, apply, and rollback failures as exit 2, but live `cmd/sworn/baton.go` returns 0 after check-mode drift and 1 for vendor errors. That owning file is absent from S01 touchpoints. The upstream path also calls `WriteUpstreamPin` only after `Vendor` has already changed mapped destinations, while S01 explicitly assigns pin changes to S02; the current contract therefore does not say whether the pin participates in the same rollback boundary or whether upstream write mode is excluded.
   What to ask the implementer: Do not edit outside the current contract. Ask the Coach to route a narrow `/replan-release` that adds `cmd/sworn/baton.go` and resolves the upstream-pin boundary explicitly, including any required `internal/baton/version.go` / `internal/adopt/baton/VERSION` ownership or an explicit mode exclusion. Re-enter implementation only against that corrected spec.

2. [mechanical] §3.3 — Recovery-only restart authority needs a deterministic, tamper-checked record
   What I observed: Approach 3 says an incomplete rollback keeps mode-0700 recovery material and that a later write enters restoration-only handling, but it does not identify the durable record by which a new process discovers the snapshot, validates its path/contents, and proves exact restoration before deleting it. Return values from the failed process cannot authorize a restart.
   What to ask the implementer: Define one deterministic, path-confined recovery record/manifest; reject symlink, path-escape, missing, or tampered material; compare every destination's bytes, mode, and existence to the recorded snapshot; and test a fresh invocation that cannot report write success until exact restoration completes.

3. [memory-cited] §2.4 — The exact-pattern adapter preserves the established normative-schema byte boundary
   What I observed: The design compiles the exceptional ECMA-262 expression through a custom predicate while keeping the upstream schema document untouched. That matches the earlier Sworn vendor-parity finding that normative schema bytes must be copied and compared byte-for-byte rather than transformed as prose.
   What to ask the implementer: Acknowledge that the prior exact-byte rule applies, keep the raw v0.15 schema bytes as compiler input, and retain the proposed input-byte equality assertion before the positive/negative path matrix.
   Citation (if [memory-cited]): [[Baton v0.13.1 upgrade prerequisite and verification]]

Pins: 3 total — 1 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins (if any): 1, 2

## Summary

Pins: 3 total — 1 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins (if any): 1, 2

## Smaller flags (not pins, worth one-line acknowledgement)

- `cmd/sworn/baton.go` is also planned by S05, but S05 is still `planned` in the same serial track; no dependency edge or status change is needed. S05 must preserve the S01 exit-mapping hunk when it reaches that file.
- `sworn designfit 2026-07-15-baton-v0.15-conformance` passes all 18 slices, including S01's ratified Type-1 bootstrap decision.
- Cross-release ancestry is clean for all nine currently planned S01 files; the track adds only the two design-record commits above `release/v0.2.0`.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR The fail-closed design is strong once the narrow S01 contract correction is ratified. 3 pins + 3 flags:

1. **Own the public exit and pin transaction boundary.** Implement against the corrected S01 spec: include `cmd/sworn/baton.go`, map check drift to exit 1 and every invalid/operational/apply/rollback failure to exit 2, and follow the replanned decision for upstream pin bytes in the same all-or-restored boundary or the explicitly excluded mode.
2. **Make recovery restart-authoritative.** Persist a deterministic path-confined recovery record, reject missing/tampered/escaping material, and prove from a fresh invocation that exact bytes, modes, and existence are restored before recovery is removed or write success is possible.
3. **Preserve normative schema bytes.** Apply the established exact-byte schema rule: compile the untouched v0.15 bytes and assert compiler-input byte identity before the required path matrix.

Flags (not pins): (a) S05's later same-track ownership of `cmd/sworn/baton.go` is serial and must preserve S01's hunk; (b) the release design-fit gate passes; (c) no production-file ancestry drift exists above the release base.

§2 decisions, including the Coach-ratified Type-1 bootstrap and the exact-byte schema policy, acknowledged. §6 items 2–5 acknowledged; §6 item 1 is governed by the corrected spec.

Address pins 1–3 inline during implementation, then proceed to in_progress.

## Triage verdict

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: S01 cannot satisfy its public exit contract within its declared touchpoints, and the upstream pin transaction conflicts with its stated scope; a narrow replan must resolve both before code.
-->
