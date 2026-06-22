# Coach-loop pause: `cmd/sworn/main.go` touchpoint collision

**Date:** 2026-06-22
**Release:** 2026-06-19-safe-parallelism
**Status at capture:** coach loop PAUSED (`~/.coach/sworn/paused`)

## What halted the loop

The parallel worker for track **T3-commercial** paged while running `verify` on
slice **S07-paging** and the loop paused at 16:18. The worker summary recorded:

> verify INCONCLUSIVE on S07-paging after 2 attempts (env issue?)

That summary is **misleading**. The verifier's actual final verdict in
`~/.coach/sworn/parallel/T3-commercial.log` was **BLOCKED → planner**, correctly
diagnosed — not an environmental fault.

## Real root cause

Forward-merging `release-wt/2026-06-19-safe-parallelism` into the
`T3-commercial` track conflicts on **`cmd/sworn/main.go`**, so the verifier had
no clean tree to verify against. It called this a **track-mode invariant 4
violation** (two tracks editing the same code file).

The slice itself is fine: `S07-paging` is `implemented` on its track branch with
a finalised proof bundle. The blocker is the **merge surface**, not the code.

### It is systemic, not a one-off

- **7 of 8 tracks** in this release modify `cmd/sworn/main.go` (all except
  `T1-concurrency-core`). It is the CLI dispatch hub: every track that adds a
  subcommand edits the same `switch os.Args[1]` block in `main()`.
- The loop has been **hand-resolving `main.go` conflicts on nearly every track
  sync** — explicit "resolve cmd/sworn/main.go dispatch conflict" merges exist
  for T2-monitoring, T4-mcp, T8-memory, and the pattern recurred in the earlier
  `fidelity-layer` release (T3-leaf-gates "session 5"). T3-commercial is simply
  the track where the chronic problem finally hard-paged the loop.

## Decision (coach, 2026-06-22)

Chosen unblock path: **refactor dispatch to a registration pattern** (durable),
not a one-off touchpoint re-group.

### Why this shape is clean here

Every command already lives in its own file under `cmd/sworn/`
(`init.go`, `run.go`, `lint.go`, `ship.go`, …). Only `main.go`'s `switch` is the
shared edit surface. Introducing a command registry that each `cmd/sworn/<name>.go`
populates (via `init()` or an explicit `register()` call) reduces `main()` to a
registry lookup that **no track ever needs to edit** — removing `main.go` as a
shared edit surface for this release and future ones.

### Landing plan (needs `/replan-release 2026-06-19-safe-parallelism`)

1. Foundational slice on the integration/release-wt base: add the command
   registry + convert the existing in-tree cases to `register()` calls; `main()`
   becomes the dispatch loop + `version`/`help` handling.
2. Each in-flight track rebases and converts **its own** `case` into a
   `register()` call in **its own** `cmd/sworn/<name>.go` file — so the touchpoint
   matrix no longer collides on `main.go`.
3. Update the touchpoint matrix so `cmd/sworn/main.go` is owned solely by the
   foundational slice; track command files are each owned by their track.

## Also parked (design-gate pages from 07:54, never ack'd)

- `S07-paging` (T3-commercial) — also merge-blocked above
- `S25-memory-search` (T8-memory)
- `S30-lint-touchpoints` (T12-harness-hardening)

Other tracks kept advancing (e.g. T12/S32-designfit-decisions-gate reached
`implemented` cleanly at 16:18).

## Housekeeping done

- Removed a stray 0-byte junk file `, or PAGE:` from the repo root (a mis-pasted
  verifier status-block fragment, created at the moment of the pause).
