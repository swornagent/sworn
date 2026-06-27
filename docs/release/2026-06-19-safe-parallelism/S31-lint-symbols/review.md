# Captain review — S31-lint-symbols
Date: 2026-06-22
Design commit: 51f6ef8fbec61080bae5c525cf09389071b3f789

## Pins

1. [mechanical] §2.1 — advisory exit code 2 collides with the existing I/O-error exit 2
   What I observed: Design §2.1 selects exit 2 for "advisory symbols found", citing
   distinction from "hard-fail exit 1". Grep `return 2` in cmd/sworn/lint.go shows
   lines 67, 73, 111, 117, 156, 162, 195, 201 already return 2 for resolveReleaseDir
   failures, stat errors, and CheckDeps/CheckTouchpoints infrastructure failures. The
   established convention is: 0 = clean, 1 = lint violation, 2 = I/O error, 64 = usage.
   A caller receiving exit 2 from `sworn lint symbols` cannot distinguish "found
   advisory warnings" from "couldn't read design.md".
   What to ask the implementer: Assign exit 3 for "advisory symbols found", keeping
   exit 2 for I/O errors — OR explicitly document that exit 2 is "advisory or I/O"
   with stderr context distinguishing them. Update design §2.1 inline before writing
   cmdLintSymbols.

2. [mechanical] §3/§2.2 — grep root not derivable from single sliceDir parameter
   What I observed: Design §3 specifies `CheckSymbols(sliceDir string) error` (one
   parameter). Design §2.2 says "grep against the repo working tree (excluding docs/)".
   The sliceDir is `<cwd>/docs/release/<release>/<slice-id>/` — deep inside docs/.
   Neither `os.Getwd()` (would grep real repo root in tests, not the fixture) nor
   walking up to find .git (temp fixture trees have none) works for the §5 test
   scenario, which creates a temp fixture tree. CheckTouchpoints uses
   `(sliceDir, releaseDir string)` as its two-parameter shape. The grep root belongs
   as a second parameter.
   What to ask the implementer: Change signature to `CheckSymbols(sliceDir, repoRoot
   string) error`. Caller in cmdLintSymbols passes the cwd (same source as
   resolveReleaseDir). Tests pass the fixture root directly. Apply inline.

3. [mechanical] §status.json — design_decisions absent from status.json
   What I observed: S31's status.json has no `design_decisions` field despite 5
   decisions in design.md §2. S29 and S30 both encode their decisions there.
   designfit.Run() line 126–129 silently skips slices with empty design_decisions
   (no gate failure), but the T12 convention is to encode decisions before code.
   All 5 are Type-2 and need no human ack.
   What to ask the implementer: Populate design_decisions in status.json from design
   §2 (5 Type-2 entries) before transitioning to in_progress.

## Summary

Pins: 3 total — 3 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none — Pin 2 (grep root) will likely cause TestSymbolsResolvedQuiet
to produce wrong results if the fixture root isn't explicitly passed; Pin 1 makes
the exit code convention inconsistent for CI callers. Neither causes a compile error
or a spec AC to silently miss.

## Smaller flags (not pins, worth one-line ack)

(a) The cmdLint usage strings (error message and usage line) currently list
    `ac|trace|deps|touchpoints` — update both the default-case error and the top-level
    usage to include `symbols` when wiring in cmdLintSymbols.
(b) The snake_case regex `\b[a-z]+(_[a-z0-9]+)+\b` requires at least one underscore,
    so single-word lowercase identifiers won't be extracted — this is intentional per
    the over-extraction mitigation; worth noting in a code comment so future maintainers
    understand the deliberate gap.

## Suggested ack reply

TL;DR clean design, 3 mechanical pins to apply inline. 3 pins + 2 flags:

1. **Exit code 3 for advisory.** The existing dispatch layer uses exit 2 for I/O
   errors (`return 2` at lines 67, 73, 111, 117, 156, 162, 195, 201 in lint.go);
   using exit 2 for advisory warnings collapses two distinct failure modes. Use exit 3
   for "unresolved symbols found" and keep exit 2 for I/O errors in cmdLintSymbols.
   Update design §2.1 inline.

2. **Add repoRoot parameter to CheckSymbols.** Change signature to
   `CheckSymbols(sliceDir, repoRoot string) error`. The cmdLintSymbols caller passes
   the cwd (already available via resolveReleaseDir's cwd logic). Tests pass the
   temp fixture root directly — this is the only way to make TestSymbolsResolvedQuiet
   grep the fixture tree rather than the real repo.

3. **Populate design_decisions in status.json.** Add the 5 Type-2 entries from
   design §2 to status.json before setting state to in_progress. All Type-2, no human
   ack needed.

Flags: (a) update cmdLint usage strings to include `symbols`; (b) add a comment to
the snake_case regex noting single-word lowercase identifiers are intentionally excluded.

§2 decisions all Type-2, no memory conflicts. §6 none.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: all 3 pins are apply-inline mechanical corrections (exit code choice, function signature, status.json field); no material design change required before code is safe
-->
