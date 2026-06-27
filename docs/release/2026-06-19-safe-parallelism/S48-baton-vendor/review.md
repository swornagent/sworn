# Captain review — S48-baton-vendor
Date: 2026-06-23
Design commit: 61d741100ca12a796943d32ede9e98b060eb7191

## Pins

1. [mechanical] §3/status.json — `cmd/sworn/main.go` in `planned_files` contradicts spec and design
   What I observed: `status.json` `planned_files` includes `"cmd/sworn/main.go"`. The spec entry point says explicitly "Does NOT edit `cmd/sworn/main.go`." Design §4 says "Does NOT edit `main.go` or `cmd/sworn/commands.go`." The file appears nowhere in design §3 either.
   What to ask the implementer: Remove `"cmd/sworn/main.go"` from `planned_files` in status.json before transitioning to in_progress. This is the same Gate 2 failure pattern that appeared in S19 and S21 (see trial log). The S30 touchpoint linter compares `planned_files` against `actual_files`; a declared file that isn't edited causes a FAIL.

2. [mechanical] §4/NOT-doing — Rule 2 deferral for network fetch tracks to wrong slice
   What I observed: Design §4 defers "network fetch of a Baton tag" with tracking: "S49-baton-version." S49's spec is scoped to "SHA→semver reconciliation and version surfacing" (and it reads the version through the `internal/baton` package S48 introduces). Network fetch is not in S49's scope. Rule 2 requires Why + Tracking + Acknowledgement — Why is present; Tracking points to the wrong slice.
   What to ask the implementer: File a GitHub issue for "network fetch support for `sworn baton vendor`" before code. Reference the returned issue number in the deferral comment in `source.go` (the hook location). Update design §4 tracking from "S49-baton-version" to the issue number.

3. [memory-cited] §1/§2 — design aligns with [[project_baton_sworn_architecture]] vendor-down flow
   What I observed: §1 transform strips bash/node script refs → sworn-native commands across rules AND prompts (the six-reference table). Decision 1 (single-table derive-both) mirrors the spec's Risks mitigation. Decision 5 (init() self-register) follows the T15/S51 pattern confirmed in the codebase.
   What to ask the implementer: Ack confirms design is consistent with [[project_baton_sworn_architecture]]'s vendor-down flow and the T15 registry pattern. No action required — confirmation only.
   Citation: [[project_baton_sworn_architecture]]

## Summary

Pins: 3 total — 2 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: Pin 1 (main.go in planned_files causes Gate 2 FAIL at verify if not corrected before code).

## Smaller flags (not pins, worth one-line ack)

(a) `design_decisions` field absent from status.json — 5 §2 decisions exist in design.md, none appear to be Type-1, so designfit gate passes trivially. S32-designfit-decisions-gate established that decisions should be populated; implementer should add all 5 with Type-2 classification before transitioning to implemented.
(b) Forward handoff to S50: `cmd/sworn/baton.go` is also in S50-baton-governance's planned_files (S50 adds `sworn baton diff` to the same file). S50 depends_on S48; sequencing is safe. Worth a one-line comment in baton.go naming this forward handoff for S50's implementer.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is sound and ready to implement. 3 pins + 2 flags:

1. **Remove main.go from planned_files.** `status.json` `planned_files` lists `"cmd/sworn/main.go"` but spec and design both say it's NOT touched. Remove it before transitioning to in_progress — it will cause the touchpoint linter (S30 Gate 2) to fail at verify.
2. **Fix Rule 2 deferral tracking for network fetch.** Design §4 tracks "network fetch" deferral to S49-baton-version, but S49 is version surfacing — not network fetch. File a GitHub issue for "network fetch support for `sworn baton vendor`", reference the issue number in the `source.go` hook comment, and update §4's tracking reference.
3. **[[project_baton_sworn_architecture]] memory-cited.** The vendor-down flow, transform map, and registry pattern align with the recorded architecture. Ack confirms — no action.

Flags (not pins): (a) Populate `design_decisions` in status.json with the 5 §2 decisions as Type-2 before transitioning to implemented (S32 gate expects it); (b) add a one-line forward-handoff comment in `baton.go` for S50's `sworn baton diff` extension.

§2 decisions 1–5 ack as Type-2. §6 empty — ack.

Address pins 1–2 inline before writing code, pin 3 is ack-only. Proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: all 3 pins are apply-inline corrections (one status.json field, one Rule 2 tracking fix, one memory-cited confirmation); none require a design re-check before code
-->
