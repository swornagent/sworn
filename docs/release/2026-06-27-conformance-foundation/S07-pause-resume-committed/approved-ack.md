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
