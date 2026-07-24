# Coach-loop parity archaeology baseline

Date: 2026-07-24
Status: corrected after Captain `REVISE`
Branch: `prep/v0.3.0-coach-parity`
Revision base: `9bb29e7c33a782a797c503a23c3ee25da9dee026`

## Authority and evidence discipline

The ratified authority is the complete Baton course-correction capture:

`/home/brad/projects/baton/docs/captures/2026-07-22-baton-v1-course-correction.md`

The target acceptance authority is:

`docs/captures/2026-07-24-sworn-v0.3-greenfield-scope.md`

The strict pre-Sworn cutoff is Fired commit `386d4589`. The current
`baton-install-backup` snapshot is an accreted endpoint, not a baseline.

Exact Fired evidence inspected with `git show`:

- `e984d658`: the four `baton/role-prompts/*.md` files; `commands/implement-slice.md`,
  `merge-track.md`, and `merge-release.md`; `bin/lib/release-board.mjs`;
  `bin/release-board-status.sh`; and `bin/release-board-ui.mjs`.
- `5d836ed6`: the `bin/coach` and `bin/coach-loop` change that removed tmux and
  dispatched fresh inline role processes.
- `2c8ce241`: changes to the release index template, Planner prompt,
  `merge-track.md`, and `verify-slice.md` separating authored plan membership
  from authoritative `status.json` state and derived views.
- `0c7b1460`: the `bin/coach-loop` and `commands/review-tldr.md` ACK/DECLINE
  triage change. This is later behaviour, not the canonical Captain contract.
- `b7654a30`: `baton/runtime-drivers.md` and
  `bin/bats/test_dispatch_driver_contract.bats`, which define and exercise one
  common process driver boundary.
- `124265bd`: commit metadata and `baton/runtime-drivers.md`; this is
  multi-provider compatibility evidence, not a source fork point.
- `386d4589`: the exact product-split change in
  `apps/docs/content/docs/captures/2026-06-13-baton-mvp-launch-spec.md`.

The lineage meaning and recovery choice below come from the ratified capture;
they are not inferred from filenames. Historical timing, token, retry, and
quality baselines remain unknown until a preserved run is executed. Fired
release documentation, when needed, must be resolved under the canonical
`/home/brad/projects/fired/apps/docs/content/docs/releases` path; the former
root `docs` path may be a symlink or branch-dependent.

## Ratified lineage and recovery formula

| Commit | Ratified contribution | Treatment |
|---|---|---|
| `e984d658` (27 May) | Earliest recoverable five-responsibility loop, role prompts, commands, Git oracle, terminal board, read-only WebUI | Architecture evidence |
| `5d836ed6` (29 May) | Removes tmux; each responsibility is a fresh inline dispatch | Process-isolation evidence |
| `2c8ce241` (29 May) | Stateless board; authored plans separated from sole machine-authoritative work state | Baseline semantics |
| `0c7b1460` (30 May) | Calibrated autonomous Captain triage | Evidence only; do not restore ACK/DECLINE as the Captain contract |
| `b7654a30` (10 June) | One role-independent runtime-driver contract for every responsibility | Baseline boundary |
| `124265bd` (14 June) | Fuller multi-provider and model-rotation behaviour | Compatibility evidence only |
| `386d4589` (14 June) | SwornAgent name and Baton/Sworn product split ratified | Strict cutoff; exclude it and everything after |

Recovery formula:

> Rebuild the May 29 stateless loop and board semantics, add only the June 10
> common role-independent driver boundary, and treat surrounding code as
> archaeology.

There is no pristine fork commit and no permission to copy the historical Bash
loop into the greenfield kernel.

## Canonical responsibility flow

```text
Planner -> proposed bounded plan -> external authorization
        -> Implementer design -> stop
        -> Captain
             PROCEED  -> resumed Implementer build + exact evidence
             REVISE   -> resumed Implementer design revision
             ESCALATE -> human decision or authorized replan
        -> fresh, read-only Verifier
             PASS       -> exact composition / Merge
             FAIL       -> Implementer repair
             BLOCKED    -> Planner or human specification decision
             no verdict -> operational retry or attention
        -> Merge only the exact passed candidate
```

Captain returns only `PROCEED`, `REVISE`, or `ESCALATE` against the design
revision. Late `/review-tldr` ACK/DECLINE handling is not the role baseline.
`BLOCKED` means the specification or authority must change; timeout, transport
failure, or process death produces no verdict. Verifier freshness is a clean
context with no implementation transcript, and read-only containment is
mandatory.

For Sworn multi-track delivery, exact passed track candidates compose serially
into one immutable assembly candidate. A distinct fresh, read-only assembly
Verifier must pass that exact commit before exact Merge to the unchanged target.

## Baseline parity boundary

Carry forward:

- bounded plan authority; Implementer design/build separation; Captain design
  decision; fresh adversarial verification; exact composition and Merge;
- committed `status` plus Git facts as delivery truth, with authored plan
  membership separate and terminal/WebUI views sharing one read-only oracle;
- one common driver contract with per-role runner and optional model selection;
- operational failure distinct from Baton verdict, and restart recovery through
  durable, idempotent command/effect paths.

Do not treat later active mission-control actions, large provider scripts,
model rotation, platform copies, ntfy, memory-search, or extra artefacts as
baseline parity. The v0.3 cockpit, provider adapters, evaluation, and optional
telemetry remain Sworn product scope, but must be rebuilt behind the greenfield
boundaries rather than justified as historical essence.

## Mapping of all nine v0.3 acceptance gates

| Gate | Required parity proof | Concrete evidence |
|---|---|---|
| 1 | Real binary passes Baton autonomous-engine cases | Case IDs, binary/version/digest, pass/fail totals, and zero false protocol verdicts |
| 2 | Real multi-track repo integrates unattended | Track/candidate/assembly/target commits and trees; final tree equals exact passed assembly |
| 3 | Every named driver passes shared corpus and configured live smoke | Driver, configured and observed model, case totals, transport result, and nullable usage/cost |
| 4 | Verifier freshness and read-only containment | Fresh invocation receipt, immutable candidate/proof digests, and zero verifier writes |
| 5 | Recovery covers every external-effect boundary | Fault point matrix, effect/receipt identities, retry counts, and zero duplicate uncertain effects |
| 6 | Terminal and WebUI stay truthful through restart/failure | Both views match the same Baton projection and durable runtime snapshot before and after restart |
| 7 | Telemetry cannot affect delivery | Identical candidate, verdict, Merge result, and exit status with export disabled, failing, and backpressured |
| 8 | Useful Coach parity matrix has no unratified gap | Every row links ratified authority plus executable evidence, or an explicit authorized deferral |
| 9 | Baseline delta is measured | Elapsed/orchestration time, protocol tokens, artefact count, retries, and quality with denominators |

Gate 9 values are recorded per scenario and in aggregate. Missing provider
usage, cost, or historical baselines are `unknown`/null, never numeric zero.
Zero is reserved for an observed zero.

## Executable S7 parity scenarios

All scenarios run through the real `sworn` binary against disposable fixture
repositories. Evidence binds the Baton package/version, Sworn binary/version,
scenario input digest, configured and observed driver/model, exact commits and
trees, invocation/command/effect receipts, verdicts, timings, retries, and
nullable usage/cost.

1. **Timeout / no verdict.** Force a role process past its configured deadline.
   Expect bounded termination and operational `NO_VERDICT`, with no `PASS`,
   `FAIL`, or `BLOCKED`. Measure deadline, observed duration, exit/signal,
   attempts, and zero protocol verdicts.
2. **Process death recovery.** Kill Sworn at every journaled external-effect
   edge, restart, and reconcile. Expect the same command receipt on replay, no
   blind retry while outcome is uncertain, and eventual progress or explicit
   operational attention. Measure recovery latency and unique effect executions.
3. **Stale target.** Move the target after assembly `PASS` but before Merge.
   Expect compare-and-swap refusal, no Merge receipt, and no target mutation by
   Sworn. Record expected and observed target commits and exactly one refusal.
4. **Verifier `FAIL` and repair.** Verify candidate C1 as `FAIL`, resume its
   Implementer to produce C2, and invoke a fresh Verifier. Expect only C2 to
   become eligible after `PASS`; C1 never composes. Record both candidate/proof
   digests, two fresh invocation IDs, repair duration, and verdict sequence.
5. **`BLOCKED` specification.** Give the Verifier an acceptance ambiguity that
   cannot be decided from current authority. Expect `BLOCKED` routed to Planner
   or human, no automatic repair/retry, and no Merge. Record blocker bytes and
   zero runtime-failure classification.
6. **Composition conflict.** Make two otherwise ready tracks modify the same
   composition hunk. Expect serial composition to stop, preserve the prior
   release ref, record conflicting paths, and skip assembly verification and
   Merge. Measure one conflict receipt and zero release-ref advances.
7. **Fresh assembly verification.** Compose at least two exact passed track
   candidates, including one dependency edge. Expect a new immutable assembly
   commit, a distinct clean read-only Verifier invocation, then Merge of exactly
   that passed commit. Record track, assembly, verifier, and final target
   identities; verifier write count must be zero.
8. **Multi-driver and per-role models.** Complete an unattended multi-track run
   with at least two driver families and different configured role models using
   the same driver contract. Expect each receipt's observed driver/model to
   match configuration with no silent fallback. Report corpus cases per
   driver/model; unreported tokens/cost stay unknown.
9. **Truthful restart views.** Capture terminal and WebUI projections, kill
   Sworn with active and uncertain work, restart, and reconnect from snapshot
   plus event offset. Expect identical Baton stage/status/role/outcome and
   durable runtime facts, then only evidence-backed advancement. Measure zero
   phantom transitions and zero view disagreements.
10. **Telemetry non-interference.** Repeat the same deterministic run with
    telemetry disabled, exporter failure, and sustained backpressure. Expect
    identical candidate/assembly/target digests, verdicts, receipts, and exit
    status. Record dropped/exported counts and delivery delta of zero; unknown
    telemetry/provider values remain null.

## Remaining evidence gaps

- No runnable preserved historical dataset yet supplies Gate 9 timing, token,
  retry, artefact, or quality numbers.
- The required live-smoke evidence remains credential-gated and must be
  reported per configured driver without silent substitution.
- Exact long-run thresholds are v0.3 acceptance choices unless ratified
  authority is found; archaeology alone cannot invent them.
