<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads
     everything between this heading and the next ## heading (or EOF). Verbatim-pasteable into the
     Implementer session — no surrounding prose. -->

TL;DR Sound design — approach (read committed planned_files, block within-phase, retry via the
S04 phase barrier) is correct and AC-traced. 4 mechanical pins + 3 flags, all apply-inline:

1. **AC-5 test seam (critical).** Your git-show reader has no injection point for the "mock
   oracle" the spec demands, and parallel.go unit tests run outside a real repo. board.SliceState
   doesn't carry planned_files and OracleReader doesn't expose it. Add a seam following this file's
   own idiom — e.g. `PlannedFilesFn func(ctx, trackID) ([]string, error)` on ParallelOptions,
   defaulting to the git-show reader — so TestInvariant2_* inject overlap/disjoint without real git.
2. **index.md parsing target.** The DOCUMENTED SHARED matrix is in the markdown BODY (lines
   76–100), not the frontmatter (ends line 49). Parse `string(indexData)` directly; don't extend
   extractFrontmatter — it discards the body.
3. **status.json design_decisions.** Add the `design_decisions` field (currently absent) and
   classify all five choices Type-2 with rationale, so the Rule 9 design-fit gate has something to
   check.
4. **Error-string assertion.** Spec gives two forms of the message (in-scope bullet vs AC-1);
   assert the shared prefix through "both write <file>" so message and test can't drift.

Flags (not pins): (a) the `repo` var at parallel.go:175 is scoped inside `if opts.Router == nil` —
not reusable by a helper; construct `git.New(absRoot)` in the helper; (b) DOCUMENTED SHARED rows
carry extra text ("oai.go + drivers") so a first-backtick parser catches one path/row — fine for
parallel.go's scope, note the limitation; (c) no §6 — open items folded into self-resolved design
risks, none needs a Coach call.

§2 decisions 1–5 acknowledged (no memory to cite — project memory is empty). No §6 questions —
acknowledged.

Address pins 1–4 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Sound, AC-traced design; all 4 pins are apply-inline mechanical corrections (test seam, parse target, design_decisions field, message-string assertion) the implementer fixes in one pass with the Verifier as backstop. No spec deviation, no judgement call, no constitutional domain.
-->
