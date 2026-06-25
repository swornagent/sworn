# Captain review — S10-provider-foundation
Date: 2026-06-23
Design commit: dd4310dbf63fb10b326da3099aaaf037ae3e5817

## Pins

```
1. [mechanical] §3 file plan / spec Planned touchpoints — ADR file number collision (CRITICAL)
   What I observed: spec and design both name `docs/adr/0004-dep-policy-minimal-justified.md`.
   ADR 0004 is already taken on this branch by `0004-tui-deps-bubbletea-lipgloss.md`
   (ADRs 0001–0006 and 0008 are present; next free is 0007). Index.md explicitly flags this:
   "S10→0007, S21→0008, after this replan's 0006." Both spec.md Planned touchpoints and
   status.json planned_files declare the old name and will diverge from the actual file.
   What to ask the implementer: Rename the ADR path to `docs/adr/0007-dep-policy-minimal-justified.md`
   in spec.md Planned touchpoints, status.json planned_files, and design §3. Also update
   CLAUDE.md's planned reference to ADR-0004 → ADR-0007.

2. [mechanical] status.json `planned_files` — three spec-declared touchpoints absent (CRITICAL)
   What I observed: spec.md "Planned touchpoints" lists `internal/model/errors.go`,
   `internal/model/errors_test.go`, and `internal/model/oai.go` (modify). status.json
   `planned_files` does NOT include any of these. S30-lint-touchpoints is a merge gate
   that fails closed on any file touched but not in planned_files — all three are load-bearing
   deliverables for this slice's ACs.
   What to ask the implementer: Add `internal/model/errors.go`, `internal/model/errors_test.go`,
   and `internal/model/oai.go` to status.json `planned_files` before writing any code.

3. [mechanical] §2.1 / §3 file plan — `internal/model/config.go` undeclared touchpoint (CRITICAL)
   What I observed: Design decision #1 and §3 explicitly commit to modifying
   `internal/model/config.go` ("FromEnv() refactoring — internal/model/config.go (modify)").
   The file appears in neither spec.md "Planned touchpoints" nor status.json `planned_files`.
   S30-lint-touchpoints will BLOCKED-fail if config.go is touched without being declared.
   What to ask the implementer: Add `internal/model/config.go` to both spec.md "Planned
   touchpoints" and status.json `planned_files`.

4. [mechanical] §2.3 / spec Risks #3 — risk mitigation requires ADR body, not code comment
   What I observed: Spec Risk #3 mitigation: "Document in ADR-0004 as an acknowledged
   trade-off of convention-based loading." Design decision #3 says "Code comment documents
   this explicitly." A code comment alone doesn't satisfy the spec risk mitigation — the trade-off
   must appear in the ADR body.
   What to ask the implementer: Confirm the ADR body (now 0007) includes the CWD .env
   injection trade-off explicitly. A code comment is fine in addition; the ADR entry is required.

5. [memory-cited] §3 ADR decision — dep policy ADR aligns with [[project-dep-policy]]
   What I observed: Memory [[project-dep-policy]] records the dep policy revision to
   "minimal, justified deps — each new dependency requires an ADR entry" confirmed 2026-06-20.
   Design's ADR-0007 (renamed from 0004) intent matches this record exactly.
   Citation: [[project-dep-policy]]

6. [memory-cited] §2 decision #4 / errors.go plan — error taxonomy aligns with [[project_provider_error_taxonomy]]
   What I observed: Memory [[project_provider_error_taxonomy]] records the 2026-06-21 Coach
   decision: model.Error{Kind} with KindAuth/Credits/RateLimit/Upstream/Transient, ClassifyHTTP,
   IsTerminal, IsTransient, UserMessage() landing in S10. Design decision #4 and §3 errors.go/oai.go
   plan align exactly with this record.
   Citation: [[project_provider_error_taxonomy]]
```

## Summary

Pins: 6 total — 4 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins (would cause slice to ship broken if unaddressed): 1, 2, 3

## Smaller flags (not pins, worth one-line ack)

- **`internal/model/oai.go` has two non-2xx return sites** (lines 188 and 250 — Verify() and Chat() respectively). The memory cited oai.go:181-182 but the actual lines differ; both paths need the `*model.Error` treatment. Design §3 covers this correctly.
- **CLAUDE.md recent WIP checkpoint commit** (`012c582`) touched the file during S18 auto-commit. Current text still says "zero runtime dependencies — stdlib only" — the intended edit is still pending and unblocked.
- **Load order for §5 `TestLoadDotEnv_CWDWins`**: The spec's Required Tests section explicitly invites the implementer to pick load order and document it. Design decision #3 (CWD first) is a valid and correctly reasoned choice; the "In scope" bullet saying "home first" is superseded by the test section. No pin.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is sound — 4 mechanical pins + 2 memory acks. Address the 3 critical admin fixes (pins 1–3) before writing any code, then apply pin 4 inline while writing the ADR:

1. **ADR rename (critical).** Rename the ADR path to `docs/adr/0007-dep-policy-minimal-justified.md` everywhere it appears: spec.md Planned touchpoints, status.json planned_files, design.md §3, and the CLAUDE.md updated text reference (ADR-0004 → ADR-0007). ADRs 0001–0006 and 0008 are present on this branch; 0007 is the next free number.

2. **Extend status.json planned_files (critical).** Add `internal/model/errors.go`, `internal/model/errors_test.go`, and `internal/model/oai.go` to status.json `planned_files`. All three are spec-declared Planned touchpoints; omitting them causes S30-lint-touchpoints to BLOCKED-fail at verify.

3. **Add `internal/model/config.go` to both spec and status.json (critical).** Design §3 and decision #1 commit to modifying config.go (FromEnv() refactoring). It appears in neither spec.md Planned touchpoints nor status.json planned_files. Add it to both.

4. **ADR body must include CWD .env trade-off.** Spec Risk #3 mitigation requires the "local .env may inject unexpected keys" trade-off to be documented in the ADR body — not only in a code comment. Include a brief "acknowledged trade-off" section in ADR-0007.

Flags (not pins): (a) oai.go has two non-2xx return sites (Verify + Chat paths, ~lines 188 and 250); both need `*model.Error` treatment; (b) CLAUDE.md WIP checkpoint is cosmetic — the target text is still the "zero runtime deps" line, no conflict.

§2 decisions ack: D1 [[project_provider_error_taxonomy]] cited — aligns with Coach decision; D3 load-order choice is spec-blessed (Required Tests explicitly delegates to implementer); D4 [[project_provider_error_taxonomy]] cited — aligns exactly; D5 [[project-dep-policy]] cited — aligns with 2026-06-20 revision. §6 question: none declared (ack empty).

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All critical pins are apply-inline admin fixes (rename ADR, extend planned_files × 2). No redesign needed; the design is architecturally sound; Verifier backstops.
-->
