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
