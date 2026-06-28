# Captain review — S07-pause-resume-committed
Date: 2026-06-28
Design commit: a45a6a1f378ebf1fb342e0d503ad0bdaf081a5b4

## Pins

1. [mechanical] §2.2 / Files-touched — **CRITICAL: the oracle is NOT "already wired into WorkerOptions"; it must be hoisted and populated at BOTH construction sites, or production silently keeps `slices[0]`.**
   What I observed: design §2 and status.json DD-3 say the oracle is "already constructed and wired into WorkerOptions (parallel.go:183/275/337)". Live repo: `ora` is built at `internal/run/parallel.go:183` **inside the `if opts.Router == nil` branch**, immediately embedded into the private `productionSliceRouter{oracle: ora}`, and is **block-scoped** — it is not in lexical scope at either `WorkerOptions` construction site (parallel.go:275 AND :337), and `WorkerOptions` has no `Oracle` field today (only `Router SliceRouter`, worker.go:97). In the injected-router path (tests, or any caller supplying `opts.Router`) `ora` is never built at all.
   What to ask the implementer: add `Oracle router.OracleReader` to `WorkerOptions`; hoist `ora` to function scope in `internal/run/parallel.go` and set `WorkerOptions.Oracle` at **both** 275 and 337; confirm the production (auto-constructed router) path yields a non-nil Oracle. If the design's "already wired" premise is taken at face value and Oracle stays nil, `findFirstNonTerminal` falls back to `slices[0]` on every production run — AC1/AC2/AC4 all silently degrade to legacy while the mock-oracle unit tests still pass.

2. [mechanical] §5 / Required tests — **CRITICAL (Rule 1 reachability): no test proves the production integration point wires a non-nil Oracle.**
   What I observed: the required unit tests (`TestFindFirstNonTerminalCommitted`, `TestFindFirstNonTerminalAllTerminalMergesTrack`) hand a mock oracle directly to `findFirstNonTerminal`. None assert that the integration point that owns the resume affordance (`internal/run/parallel.go`) actually populates `WorkerOptions.Oracle`. So design §4's claim that "every `--parallel` run reads committed state" (and AC6's observable contract) is unproven at the integration point — the leaf function is tested in isolation.
   What to ask the implementer: add a reachability assertion (or explicit smoke step) that the production construction path wires `Oracle` non-nil, so AC1's committed-read holds in the real resume path and not only in the unit that supplies the oracle. This is the Rule 1 artefact for the slice.

3. [mechanical] §2.2 / Files-touched — **Wrong path citations: `internal/scheduler/run_parallel.go` and `internal/scheduler/model.go` do not exist.**
   What I observed: design cites "`WorkerOptions` is constructed in `internal/scheduler/run_parallel.go`" and "The `SliceRouter` interface (`internal/scheduler/model.go`)". Live: `WorkerOptions` is constructed in `internal/run/parallel.go` (275/337) — a different package (`internal/run`, not `internal/scheduler`); `SliceRouter` is defined in `internal/scheduler/worker.go:48`. Neither `run_parallel.go` nor `model.go` exists under `internal/scheduler/`. (The status.json line numbers 275/337 do match `internal/run/parallel.go`.)
   What to ask the implementer: re-anchor the citations to `internal/run/parallel.go` and `internal/scheduler/worker.go` before coding, so the oracle-threading plumbing lands in the right package. Note the construction site is in `internal/run`, so the wiring change is an `internal/run` edit, not an `internal/scheduler` one.

4. [mechanical] §"Import cycle risk" — **Rationale is wrong: scheduler does NOT currently import `internal/router`; the new edge is genuinely new (but still safe).**
   What I observed: design says "`internal/scheduler` already imports `internal/router` (`SliceRouter` is `router.Router`)". Live: the scheduler does **not** import `internal/router` (worker.go:46 is a comment only); `SliceRouter` is a scheduler-local interface (worker.go:48) and `SliceDecision` is a scheduler-local type — a deliberate decoupling, with `internal/run`'s `productionSliceRouter` wiring the real router across the boundary. Adding `router.IsTerminal` + `router.OracleReader` to the scheduler therefore introduces a **new** `scheduler → router` import edge.
   What to ask the implementer: the design's conclusion still holds — `internal/router` imports only `board`+`git` (not `scheduler`), so no cycle is created — but drop the "type-aliased to avoid a circular import" hedge; there is no cycle risk to avoid. The new edge couples scheduler→router directly; DD-2 (Type-1, Coach-ratified) already blesses the `IsTerminal` coupling, so it is spec-sanctioned. If preserving the `SliceRouter`-style decoupling matters, threading a scheduler-local `OracleReader` interface (mirroring `SliceRouter`) instead of `router.OracleReader` is the pattern-consistent alternative (Type-2, implementer's call).

5. [mechanical] §2.2 / AC3 — **AC3's nonexistent-ref scenario is already satisfied by the oracle; the design's residual "skip on error" risks re-introducing the forward-only abandonment this slice exists to prevent.**
   What I observed: I determined the AC3 fact from live code. `CatFileExists` (`internal/git/git.go:108`) swallows a missing-ref `git cat-file -e` failure to `(false, nil)`; both `resolvePrefix` (oracle.go) and `readSliceStatusFromRef` (oracle.go) route through it, so a nonexistent track ref returns empty — not an error — and falls through to the release-wt priority **inside** `Oracle.ReadSliceStatus`. So the common crash-recovery case (track branch not yet created) needs no new handling in `findFirstNonTerminal`. The only residual hard-error sources are `parseStatusJSON` (malformed status.json) and a transient `reader.Show` failure. The design's plan for a hard read error is "skip the slice and continue to next" — but skipping a slice whose committed state is unknown lands the frontier on a *later* slice the forward-only router can never return to, which is exactly the abandonment AC2/DD-1 exists to prevent.
   What to ask the implementer: AC3's explicit nonexistent-ref path is already covered (confirm via the `CatFileExists` swallow), so the AC3 test should exercise it through the real oracle chain, not just a mock that returns an arbitrary error. For the residual hard-error case (malformed content), prefer **seeding at** the unreadable slice (return it) over **skipping past** it, to stay consistent with the slice's no-abandonment thesis — or document why skip is safe.

## Summary

Pins: 5 total — 5 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (would ship broken if unaddressed): 1 (oracle never wired → AC1/AC2/AC4 silently degrade to legacy in production), 2 (no integration-point test proves the wiring, so the degradation ships green).

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) The design "Approach" has no explicit §1 "user-visible change" paragraph, but the AC-traceability table at the end maps all six ACs to implementation — coverage is complete, acceptable.
- (b) The `--resume` usage gate (`if *resume && !*parallel { return 64 }`) mirrors the existing `--dry-run` gate at `cmd/sworn/run.go:65` — good precedent, pattern-consistent.
- (c) Design §4 calls `--resume` "observational" (no code-path change). AC6 forbids a "silent no-op flag" and requires a "stated observable effect". The design satisfies this via help text ("on every `--parallel` run each track seeds from committed state; `--resume` is an explicit alias"). Confirm that help-text wording actually ships, so the flag is documented rather than a silent no-op.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Sound, Coach-ratified approach (DD-1/2/3); the three surgical changes are right. The pins are all apply-inline corrections to inaccurate code citations — none change the design. 5 pins + 3 flags:

1. **Oracle wiring (CRITICAL).** The oracle is NOT already wired into `WorkerOptions` as §2/DD-3 state. `ora` is block-scoped inside `if opts.Router == nil` at `internal/run/parallel.go:183` and embedded in the private `productionSliceRouter`; `WorkerOptions` has no `Oracle` field. Add `Oracle router.OracleReader` to `WorkerOptions`, hoist `ora` to function scope, and populate `WorkerOptions.Oracle` at **both** construction sites (parallel.go:275 AND :337). Confirm production yields a non-nil Oracle — otherwise `findFirstNonTerminal` silently falls back to `slices[0]` and AC1/AC2/AC4 degrade to legacy while the mock-oracle unit tests still pass.
2. **Prove the wiring (CRITICAL, Rule 1).** The required unit tests hand a mock oracle straight to `findFirstNonTerminal`; nothing proves the production integration point (`internal/run/parallel.go`) sets `Oracle` non-nil. Add a reachability assertion (or explicit smoke step) that the production path wires `Oracle` non-nil — that is the slice's Rule 1 artefact.
3. **Fix path citations.** `internal/scheduler/run_parallel.go` and `internal/scheduler/model.go` don't exist. `WorkerOptions` is built in `internal/run/parallel.go` (275/337); `SliceRouter` lives in `internal/scheduler/worker.go:48`. Re-anchor before coding — the wiring change is an `internal/run` edit.
4. **Import-cycle rationale.** Scheduler does NOT currently import `internal/router` (worker.go:46 is a comment; `SliceRouter`/`SliceDecision` are scheduler-local). The new `scheduler → router` edge is genuinely new but safe (router imports only board+git). Drop the "type-alias to avoid a circular import" hedge — no cycle exists. DD-2 already blesses the `IsTerminal` coupling; if you want to keep the `SliceRouter` decoupling, thread a scheduler-local `OracleReader` interface instead (Type-2, your call).
5. **AC3 already covered + seed-don't-skip.** `CatFileExists` (git.go:108) swallows a missing-ref error to `(false,nil)`; `resolvePrefix` and `readSliceStatusFromRef` both fall through to release-wt inside the oracle on a nonexistent ref — AC3's explicit scenario needs no new handling, so exercise it through the real oracle chain in the AC3 test. For the residual hard-error case (malformed status.json), prefer seeding AT the unreadable slice over skipping past it (skipping re-introduces the forward-only abandonment AC2/DD-1 prevents), or document why skip is safe.

Flags (not pins): (a) no explicit §1 paragraph but the AC-traceability table covers all six ACs — fine; (b) the `--resume` usage gate correctly mirrors the existing `--dry-run` gate at run.go:65; (c) ensure the AC6 help-text wording ("committed seed on every `--parallel` run; `--resume` is an explicit alias") actually ships so the flag isn't a silent no-op.

§2 decisions DD-1 (Type-1), DD-2 (Type-1), DD-3 (Type-2) all carry recorded Coach acknowledgements in status.json — design-fit gate (Rule 9) passes; no memory entries exist to cite. §6 has no open questions; §1–5 review surfaced the pins above.

Address pins 1–5 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Approach is sound and Coach-ratified (DD-1/2/3); all 5 pins are apply-inline corrections to inaccurate code citations (incl. 2 critical-but-unambiguous oracle-wiring fixes) — none require re-reviewing the design before code, and the Verifier backstops AC1.
-->
