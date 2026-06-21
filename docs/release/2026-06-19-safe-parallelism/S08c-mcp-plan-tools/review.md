# Captain review — S08c-mcp-plan-tools (Round 2)
Date: 2026-06-21
Design commit: 756f937585c0bc3a8e589c6bd5febfb797c087f2

> **Round 2 re-review.** Round 1 (2026-06-21) surfaced 6 pins (3 mech, 1 mem, 2 esc);
> Coach declined (see `decline.md`). Implementer revised design.md to address all 6 pins
> (see design §7 Pin resolutions). This review assesses the revised design only.
>
> **Drift note:** T4-mcp track is 36 commits behind `release-wt/` (T9-telemetry,
> T11-infra-safety, T12-harness-hardening merges). These tracks are orthogonal to MCP
> tooling; S08c's spec and design are unaffected by those commits. Review proceeded.

## Round 1 pin closures (all 6 resolved)

| Pin | Resolution |
|-----|------------|
| Pin 1 server dispatch gap | §3 explicitly adds `RegisterResource`/`RegisterPrompt` + `resources/read`/`prompts/get` to server.go; design §7.1 confirms this with exact API names |
| Pin 2 baton/rules source | §4 deferral with full Rule-2 compliance (why: source not yet built; tracking: S21-canonical-baton; ack: Coach 2026-06-21) |
| Pin 3 baton/ dir missing | §3 creates `internal/prompt/baton/`; vendors `track-mode.md`; serves baton/version from existing `internal/prompt/VERSION.txt` (no duplicate) |
| Pin 4 yaml.v3 ADR | Decision 2 chooses stdlib/strings — no new dep, no ADR needed |
| Pin 5 cmd/sworn/mcp.go missing | Added to §3 and status.json planned_files; wiring point confirmed |
| Pin 6 spec reachability | §5 amended to `create_slice` demo; spec Required Tests reachability updated accordingly |

## Pins

1. `[mechanical]` §3 — spec-prescribed embed-absent error text not named in resources.go plan
   What I observed: Spec Risks says "If the embed is somehow absent... return a clear error:
   'sworn://prompts/plan: embedded prompt not found — this is a binary build error; please
   reinstall sworn.'" Design Decision 5 says "if the embed is absent or corrupted, the resource
   returns an error" but gives no message format. The spec prescribes a specific error string
   that includes the URI prefix and a "binary build error; please reinstall sworn" suffix.
   What to ask the implementer: In `resources.go`, when `embeddedFS.ReadFile(path)` returns an
   error (or the embedded file is not found), return the spec-prescribed format:
   `"sworn://<uri>: embedded prompt not found — this is a binary build error; please reinstall sworn."`
   Apply inline during implementation.

2. `[mechanical]` §3 — colon-space YAML safety test missing from test plan
   What I observed: Spec Risks prescribes "Test with a slice title containing a colon-space"
   for the `set_track` YAML frontmatter path. Design §3 names `TestSetTrackValidation` (tests
   non-existent slice_id) but does not name a colon-space test. The `set_track` handler
   manipulates raw YAML frontmatter via stdlib strings; a slice title like "My tool: setup" in
   the slices list must round-trip without corrupting the frontmatter. No test name covers this.
   What to ask the implementer: Add a test case (e.g. `TestSetTrackColon`) to `tools_test.go`
   that calls `set_track` with a slice whose ID or title contains "colon: space" and asserts
   the resulting `index.md` frontmatter is valid YAML. Apply inline during implementation.

3. `[memory-cited]` §2 Decision 2 — stdlib over yaml.v3 confirmed; §6 question resolved by memory
   What I observed: Decision 2 cites [[project_dep_policy]] for the no-yaml.v3 pick. Memory
   [[feedback_dep_justification_test]] explicitly records the Coach's decision for this exact
   slice: "S08c → DON'T add yaml.v3, use stdlib (not justified)" — sworn-authored narrow
   frontmatter, general YAML parser risks key-reordering/comment stripping, new module family
   for one function. Decision aligns. The §6 open question ("yaml.v3 vs stdlib — Confirm, or
   direct me to add yaml.v3 behind an ADR") is fully answered by this memory; no human decision
   needed. Ack confirms the citation is correct.
   Citation: [[feedback_dep_justification_test]], [[project_dep_policy]]

---

## Summary

Pins: 3 total — 2 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: None. Pin 1 applies to a defensive code path (embed absent in a correctly built
binary — should never happen); Pin 2 is a missing test name. Neither causes the feature to ship
broken, but both would surface as Verifier findings.

## Smaller flags (not pins, worth one-line ack)

(a) **S21 file overlap.** `S21-canonical-baton` (T3, planned) lists `internal/prompt/baton/track-mode.md`
    and `internal/prompt/prompt.go` in its planned_files — both also in S08c. T4-mcp merges before
    T3-commercial, so S21 will find `track-mode.md` already exists (S08c's vendored copy) and will
    overwrite it with the canonical version. `prompt.go` edits are additive and will merge cleanly.
    No action needed for S08c; note for the S21 Implementer to expect the file pre-existing.

(b) **`status.json` `test_commands` is empty.** Populate (e.g. `"go test ./internal/mcp/... -count=1 -timeout 60s"`)
    before transitioning to `in_progress`.

(c) **Forward-merge required.** T4-mcp is 36 commits behind `release-wt/`. Implementer must
    `git merge release-wt/2026-06-19-safe-parallelism` into the track branch before writing code.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Clean Round 2. All 6 Round-1 pins are resolved; 3 lightweight new pins to apply inline + 3 flags:

1. **Embed-absent error text (apply inline).** In `resources.go`, when an embedded file is not
   found, return the spec-prescribed format: `"sworn://<uri>: embedded prompt not found — this is
   a binary build error; please reinstall sworn."` (Spec Risks, first bullet.)

2. **Colon-space test (apply inline).** Add `TestSetTrackColon` (or similar) to `tools_test.go`:
   call `set_track` with a slice whose title or ID contains `"colon: space"`; assert the resulting
   `index.md` frontmatter is valid YAML. (Spec Risks, second bullet.)

3. **yaml.v3 decision (memory-cited, no change needed).** Decision 2 (stdlib over yaml.v3) aligns
   with [[feedback_dep_justification_test]] — Coach's recorded call for this exact slice. §6 yaml.v3
   question is answered; proceed with stdlib. No ADR needed.

Flags: (a) S21 will find `track-mode.md` pre-existing (expected — it overwrites with canonical);
(b) populate `test_commands` in status.json before in_progress;
(c) forward-merge `release-wt/` before writing code (36 commits ahead, orthogonal domain).

§2 decisions 1 (CreateRelease exported), 3 (update_intake append-or-create), 4 (bespoke path matching), 5 (go:embed extended) ack — sound.
§4 deferrals (baton/rules→S21, resources/list→post-R3, create_release MCP tool→S20) ack — Rule-2 compliant.
§6 open question (yaml.v3) ack — resolved by memory, no human decision needed.

Address pins 1 and 2 inline during implementation. Pin 3 is a confirmation, no code change. Proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 3 Round-2 pins are apply-inline corrections (spec error text, missing test name, memory-cited confirmation). No design rethink or Coach authority needed; the two Round-1 escalations were resolved by the Coach's decline and are properly reflected in the revised design.
-->
