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
