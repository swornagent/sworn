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
