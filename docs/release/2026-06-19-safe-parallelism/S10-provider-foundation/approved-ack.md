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
