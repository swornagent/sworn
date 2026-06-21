<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR solid benchmark design, 5 mechanical pins — 2 are critical (DB + per-track dirs). Address all 5 inline during implementation.

1. **SQLite DB init.** Before calling `RunParallel`, open an in-memory DB: `sql.Open("sqlite", ":memory:")`. Run the same supervisor schema-init step as production. Check `parallel_test.go` for the exact pattern.
2. **Pre-create per-track temp dirs.** The fixture generator must create a temp dir for EACH track's `worktree_path` entry in index.md frontmatter — not just the release worktree dir. Failure to do so triggers `git worktree add` inside the worker, which will fail (no release-wt branch) and cascade all tracks to TrackFail.
3. **5× repetition + `runs` field.** Either run 5 iterations per N value and include a `runs` field in the result struct (per spec "In scope"), or note in design §4 that single-shot is intentional. Resolve before writing the loop.
4. **Mock counter race safety.** The internal overclaim/underclaim counter is shared across goroutines. Use `sync.Mutex` or record results via a channel and accumulate after `wg.Wait()`. Verify with `go test -race`.
5. **Spec Risk mitigation ack.** Add a sentence to design §4 (or §2 D1) stating: "The mock's independence from spec content and worktree paths makes the 'similar spec content, same worktree root' fixture constraint inapplicable — accepted because the mock tests scheduler correctness, not verifier content-sensitivity."

Flags (not pins): (a) avoid naming a function `Run` in overclaim.go (conflicts with existing `bench.Run`); (b) commit the `--publish` output as part of the slice diff; (c) add `design_decisions` to status.json (vacuous for now, for S32 parity).

§2 decisions D1–D5 ack. §6 empty ack (no open questions).

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are mechanical apply-inline; 2 critical ones (DB init, per-track dirs) have deterministic fixes visible from the existing test suite; no design re-review needed before code.
-->
