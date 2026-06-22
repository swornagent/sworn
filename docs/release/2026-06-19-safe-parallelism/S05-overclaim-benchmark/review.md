# Captain review — S05-overclaim-benchmark
Date: 2026-06-22
Design commit: 333769083a7afea0d7d3f94fee2a5cb7b5dc9e02

## Pins

1. [mechanical] §4 — `RunParallel` requires a valid `*sql.DB`; design omits initialisation
   What I observed: `RunParallel` passes `opts.DB` to `scheduler.RunTrack`, which calls `supervisor.New(opts.DB, release).Acquire(trackID)` — executing SQL immediately. A nil DB panics at runtime. The design says "uses temp directories" for worktrees but says nothing about the supervisor's database requirement. Existing parallel_test.go shows the pattern: `sql.Open("sqlite", ":memory:")` with the supervisor schema init before calling `RunParallel`.
   What to ask the implementer: open an in-memory SQLite DB (`sql.Open("sqlite", ":memory:")`) before calling `RunParallel`, and run the same schema-init step that production uses. Confirm the schema-init function is exported or duplicate the CREATE TABLE call.

2. [mechanical] §4 — per-track worktree paths also need pre-created temp dirs; design only mentions the release worktree path
   What I observed: `RunParallel` pre-flight only materialises the RELEASE worktree (`release_worktree_path` from frontmatter). The worker (`scheduler.RunTrack`) separately materialises each TRACK's worktree: if `trackInfo.WorktreePath` doesn't exist on disk, it runs `git worktree add <path> -b <branch> release-wt/<release>`. For the fixture, no `release-wt/<fixture-name>` branch exists anywhere, so materialisation would fail → `TrackFail` → cascade → all tracks fail → overclaim rate ≠ 0%. Design §4 says "worktree_path in index.md points to the temp dir so RunParallel skips worktree materialisation" — but "the temp dir" reads as singular (the release dir). Per-track temp dirs are not mentioned.
   What to ask the implementer: pre-create a temp dir for EACH track's `worktree_path` entry in the fixture's index.md frontmatter (in addition to the release worktree temp dir). Fixture generator must create N+1 dirs before writing the index.md.

3. [mechanical] §1 and §3 — 5× repetition per N level and `runs` result field omitted
   What I observed: Spec "In scope" says "Repeats 5× at each N and averages (deterministic mocks → same result each time; repetitions confirm no non-determinism was introduced)" and the spec's result struct includes a `runs` field. Design §3 describes the harness as "runs RunParallel at N=1,2,4" with no mention of repetition count. §1 user-visible description is also silent on this. AC4 ("Running `sworn bench overclaim` 5× produces identical output") tests external determinism, but the spec also requires internal 5× repetition.
   What to ask the implementer: confirm whether the harness runs 5 iterations per N value (averaging them) or is single-shot. If 5×, ensure the result struct includes a `runs` field and the Markdown table shows it. If intentionally single-shot (acceptable given deterministic mocks), acknowledge the deviation from the spec "In scope" description in design.md §4 or §2.

4. [mechanical] §3 — mock RunSliceFn's internal overclaim counter is shared mutable state under concurrency
   What I observed: Design Decision D1 says the mock "internally records the ground truth and the simulated verifier verdict; overclaim/underclaim is computed from the recorded data." Under N=4, four goroutines call the same closure concurrently. If the closure captures counters as plain `int` variables (or a slice without locking), this is a data race — `go test -race` will catch it and fail AC6.
   What to ask the implementer: protect the shared counter with `sync.Mutex` or use `sync/atomic`. Alternatively, return the per-slice verdict from the mock and accumulate results in the caller after `wg.Wait()`. Confirm the design's recording approach and add the synchronisation before implementation.

5. [mechanical] §2 Decision D1 vs Spec Risk — "similar spec content, same worktree root" fixture requirement silently bypassed
   What I observed: Spec Risk mitigation says "the fixture includes at least 2 PASS slices that could be confused by a stateful verifier (similar spec content, same worktree root)." Design D1 makes the mock return nil unconditionally and read only the `owner` field — it does not look at spec content or worktree paths at all. The "similar content, same root" requirement is irrelevant to the mock's logic, but the design does not acknowledge this. Silent deviations from spec Risk mitigations fail Gate 2.
   What to ask the implementer: add an explicit acknowledgement in design.md §2 or §4 that Decision D1's mock independence from spec content/worktree paths renders the "similar spec content, same worktree root" fixture constraint inapplicable — and state why this is acceptable (the mock tests scheduler correctness, not verifier content-sensitivity). This is a documentation fix only; no code change.

## Summary

Pins: 5 total — 5 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: 1 (DB panic at runtime) and 2 (per-track materialisation cascade-fail) would cause the slice to ship broken if unaddressed.

## Smaller flags (not pins, worth one-line ack)

- `status.json` has no `design_decisions` field; S05 has no Type-1 choices, so this is vacuously fine, but worth filling in for `sworn designfit` parity once S32 lands.
- Existing `internal/bench/` package has a `bench.Run` function (model benchmark runner). The overclaim harness must use a different name (`RunOverclaimBenchmark` or similar) to avoid a compile error in the same package.
- The design's §4 says `--publish` "does not auto-commit" and defers committing to the implementer session. Ensure the spec's AC5 ("writes a valid Markdown file to docs/benchmark/...") is satisfied by committing the output file as part of the slice diff — not as a separate manual step.

## Suggested ack reply

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
