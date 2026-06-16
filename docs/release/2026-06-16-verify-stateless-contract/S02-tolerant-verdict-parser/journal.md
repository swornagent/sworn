# Journal: `S02-tolerant-verdict-parser`

## Session 2026-06-17 — initial implementation

**State transition:** `planned` → `in_progress` → `implemented`

**Decisions:**

1. **`firstVerdictLine` skips bare fence lines.** The spec's acceptance check "`` ```\nPASS ``" was interpreted as a bare code-fence line preceding the verdict. The implementation skips any line that is exactly ` ``` ` after trimming, then takes the next non-empty line as the verdict source. This is the narrowest interpretation that satisfies the acceptance check without widening the parser to skip arbitrary prefixes.

2. **`stripMarkdown` strips inline emphasis only — not the leading fence from `firstVerdictLine`.** The fence skip happens in `firstVerdictLine`: bare fence lines are skipped; inline fence markers (e.g. ` ```PASS `) are handled by `stripMarkdown` stripping the leading `` ``` ``. This two-level approach keeps each function's responsibility clear: `firstVerdictLine` picks the right line; `stripMarkdown` cleans a single line.

3. **Rationale stored as full original text.** Before this change, `parseVerdict` stored `t` (the trimmed line) as `Rationale`. Now it stores the full `text` — the complete model reply. A verifier reviewing a BLOCKED verdict needs the full context, not just the single line that failed to parse.

4. **All new tests drive through `verify.Run`** (Rule 1). No direct unit tests on `firstVerdictLine` or `stripMarkdown` — they are exercised through the integration point with `fakeVerifier` replies. This satisfies the reachability gate.

5. **Public-safe synthetic fixtures.** All canned spec/diff content is hand-written ("must do X", "+ did X"). No private dogfood slice data was imported.

6. **`blocked` helper preserved.** The existing `blocked(gate, why)` helper was kept as a one-liner alongside the new functions. No semantic change.

**Trade-offs:**
- A reply with ` ```something_else ` as the first non-empty line will have `` ``` `` stripped by `stripMarkdown`, leaving `something_else `, which will BLOCK. This is correct: we only skip a *bare* fence line.
- Emphasis stripping uses `TrimLeft`/`TrimRight` with `"*_\`"` — this means `*_PASS_*` → `PASS` but also means `__PASS__` → `PASS`. The fail-closed property (only `PASS`-leading tokens pass) prevents over-stripping from granting a false PASS.

**Out-of-scope discoveries:** None.

## Verifier verdicts received

### 2026-06-17 — fresh-context session

**Verdict: PASS**

Gates walked (all satisfied):

1. **User-reachable outcome exists** — `verify.Run` is the integration point for both `sworn verify` CLI and `sworn run`'s verify gate. All 5 new tests drive through `verify.Run`, not a private helper. ✓
2. **Planned touchpoints match actual changed files** — Planned: `internal/verify/verify.go`, `internal/verify/verify_test.go`. Actual diff (`git diff --name-only 34f7adcabc76a9f57c1119f3008391fd403f5a71`): same two files plus release artefacts. ✓
3. **Required tests exist and exercise the integration point** — All 5 acceptance-check test functions present (`TestParseVerdict_MarkdownEmphasis`, `TestParseVerdict_LeadingBlankLines`, `TestParseVerdict_LeadingFence`, `TestParseVerdict_ToolCallLeakBlocks`, `TestParseVerdict_ProsePreambleBlocks`). All green on independent run. ✓
4. **Reachability artefact proves the user path** — `go test ./internal/verify/... -v` passed (11/11 tests); E2E gate type is N/A per spec. ✓
5. **No silent deferrals or placeholder logic** — grep for TODO/FIXME/deferred/placeholder returned nothing. ✓
6. **Claimed scope matches implemented scope** — All 6 acceptance checks delivered with verifiable evidence references (file + line). ✓

`verifier_was_fresh_context: true`