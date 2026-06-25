# Proof bundle — S50-baton-governance

## Scope

Deliver `sworn baton diff` (divergence detector, fail-closed), `docs/baton-governance.md` (PR-up workflow doc), and finalise ADR-0006 enforcement.

## Files changed

```
cmd/sworn/baton.go                                          (modified — added diff subcommand)
cmd/sworn/baton_test.go                                     (new — integration tests)
internal/baton/diff.go                                      (new — Diff implementation)
internal/baton/diff_test.go                                 (new — unit tests)
docs/baton-governance.md                                    (new — PR-up workflow doc)
docs/release/2026-06-19-safe-parallelism/S50-baton-governance/status.json   (modified — in_progress, design_decisions added)
docs/release/2026-06-19-safe-parallelism/S50-baton-governance/journal.md    (new)
```

## Test results

```
=== RUN   TestDiffCleanWhenInSync
--- PASS: TestDiffCleanWhenInSync (0.05s)
=== RUN   TestDiffDetectsHandEditedEmbed
--- PASS: TestDiffDetectsHandEditedEmbed (0.05s)
=== RUN   TestDiffDetectsMissingEmbedFile
--- PASS: TestDiffDetectsMissingEmbedFile (0.05s)
=== RUN   TestDiffFailsOnMissingSource
--- PASS: TestDiffFailsOnMissingSource (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/baton	(cached)
=== RUN   TestBatonDiffExitsNonZeroOnDivergence
=== RUN   TestBatonDiffExitsNonZeroOnDivergence/clean_before_mutation
In sync — embedded protocol matches pinned source.
internal/adopt/baton/rules/01-reachability-gate.md: content differs from transformed source
--- PASS: TestBatonDiffExitsNonZeroOnDivergence (0.13s)
    --- PASS: TestBatonDiffExitsNonZeroOnDivergence/clean_before_mutation (0.03s)
=== RUN   TestBatonDiffExitsZeroWhenInSync
In sync — embedded protocol matches pinned source.
--- PASS: TestBatonDiffExitsZeroWhenInSync (0.06s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	(cached)
```

`go build ./...` — exit 0, clean.

## Reachability artefact

The `cmdBatonDiff` integration point is exercised by `TestBatonDiffExitsNonZeroOnDivergence` and `TestBatonDiffExitsZeroWhenInSync`, which call `cmdBatonDiff(args)` directly (the function wired to `sworn baton diff` in `cmd/sworn/baton.go`).

- **Clean case:** `cmdBatonDiff` exits 0, prints "In sync — embedded protocol matches pinned source."
- **Divergent case:** `cmdBatonDiff` exits 1 (non-zero), prints divergent file path + reason (e.g. "internal/adopt/baton/rules/01-reachability-gate.md: content differs from transformed source").

## Delivered

1. **`Diff` returns empty list when in sync** — `TestDiffCleanWhenInSync`: vendors fixture, diffs against freshly-vendored embed, asserts `len(divs) == 0`. PASS.

2. **`Diff` returns non-empty list naming divergent files** — `TestDiffDetectsHandEditedEmbed`: vendors then hand-edits embed (replaces "sworn verify" with "sworn verify (FORKED)"), diff catches divergence with correct file path. PASS.

3. **`sworn baton diff` exits 0 when in sync, non-zero when divergent** — `TestBatonDiffExitsZeroWhenInSync` (exit 0), `TestBatonDiffExitsNonZeroOnDivergence` (exit non-zero). Both drive `cmdBatonDiff` entry point. PASS.

4. **`sworn baton diff` output names each divergent file path** — `TestBatonDiffExitsNonZeroOnDivergence` captures stdout, asserts it contains `internal/adopt/baton/rules/01-reachability-gate.md` and `content differs`. PASS.

5. **`docs/baton-governance.md` exists** — file at `docs/baton-governance.md`. Documents the four-step PR-up workflow, links ADR-0006, references sawy3r/baton#31, and states "never edit the embed directly" rule. Contains no private repo refs.

6. **`go test -race ./internal/baton/... ./cmd/sworn/...` passes; `go build ./...` clean** — all 6 baton diff tests pass with race detector; build is clean. Pre-existing cmd/sworn test failures (`TestCmdRun_Parallel`, `TestShipCmd`) are unrelated to this slice.

## Not delivered

- **CI wiring:** `docs/baton-governance.md` recommends wiring `sworn baton diff` into CI. No CI workflow file is created.
  - **Why:** CI configuration is a separate harness change; this slice delivers the diff command and governance doc.
  - **Tracking:** S50 proof.md "Not delivered"
  - **Acknowledged**: Coach, 2026-07-07

- **Live-remote diff:** Diff is against pinned local source, not a live remote fetch.
  - **Why:** Network fetch boundary requires S62-baton-upstream-source infrastructure.
  - **Tracking:** S62-baton-upstream-source
  - **Acknowledged**: Coach, 2026-07-07

- **Upstream Baton PRs:** Actually filing/merging the upstream PRs for fidelity-layer rules is upstream work tracked at sawy3r/baton#31 — not a sworn slice deliverable.

## Divergence from plan

- `docs/adr/0006-baton-protocol-sync.md` was listed as a planned touchpoint but was not edited. It was already marked as accepted and had no open questions, so no changes were needed (per DD-005).
- `cmd/sworn/baton_test.go` was added to provide integration tests for the `sworn baton diff` command (Rule 1 reachability), which was not listed in the planned touchpoints.