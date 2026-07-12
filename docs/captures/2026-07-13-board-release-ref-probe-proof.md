# Proof bundle — board oracle release-ref probe (sworn#100)

Date: 2026-07-13
Anchor: [swornagent/sworn#100](https://github.com/swornagent/sworn/issues/100)
Base: `release/v0.1.0` @ `7e2e5c3`

## Scope

Fix the release-ref resolution so a board.json-native release (ADR-0009,
records-as-JSON) resolves to its `release-wt` ref instead of silently falling back
to HEAD and reporting the failure against the wrong ref.

## Files changed

`git diff --name-only HEAD` (plus untracked, from live repo state):

```text
cmd/sworn/board.go
cmd/sworn/route.go
internal/tui/board.go
internal/board/releaseref.go        (new)
internal/board/releaseref_test.go   (new)
```

Three duplicated probes collapsed into one shared helper: 3 files changed,
21 insertions, 33 deletions, plus the new helper and its test.

## Root cause

Two stacked defects, duplicated across three call sites.

1. **Wrong probe.** The `release-wt` ref was validated by probing for `index.md`.
   Per ADR-0009 the authoritative record is `board.json`; `index.md` is a rendered
   view that a records-as-JSON release need not carry. The probe therefore missed
   every board.json-native release. `internal/tui/board.go:442` carried a comment
   stating it "mirrors cmd/sworn/board.go's release-wt resolution" — the
   duplication was known and noted, not accidental.

2. **Silent fallback.** On a miss the ref was reassigned to `HEAD` without a word,
   and the resulting failure was reported *against HEAD*. The operator is sent
   looking for a release on the integration branch that is sitting on `release-wt`.

`sworn route` is the loop's router, so this broke `sworn run` on any board.json-native
release, not only the read-only `board` command.

## Test results

New guard — `go test ./internal/board/ -run TestReleaseRefFor -v`:

```text
--- PASS: TestReleaseRefFor_BoardJSONOnlyResolvesToReleaseWT (0.00s)
    --- PASS: .../docs/release
    --- PASS: .../apps/docs/content/docs/release
--- PASS: TestReleaseRefFor_LegacyIndexMDStillResolves (0.00s)
--- PASS: TestReleaseRefFor_NoReleaseWTBranchFallsBackToHEAD (0.00s)
--- PASS: TestReleaseRefFor_BranchWithoutRecordsFailsClosed (0.00s)
PASS
ok   github.com/swornagent/sworn/internal/board  0.012s
```

Full suite — `go test ./...`: **47 packages ok, 0 failures**. `go build ./...` and
`go vet` on the touched packages clean.

## Guard fidelity (Rule 12)

**Mutation proof.** `releaseRecords` reverted to the original defect form
(`[]string{"index.md"}`) and the suite re-run:

```text
--- FAIL: TestReleaseRefFor_BoardJSONOnlyResolvesToReleaseWT/docs/release
    releaseref_test.go:30: ReleaseRefFor: unexpected error: release
    "2026-07-11-design-system": branch refs/heads/release-wt/2026-07-11-design-system
    exists but carries no board record (probed index.md under docs/release,
    apps/docs/content/docs/release)
--- FAIL: TestReleaseRefFor_BoardJSONOnlyResolvesToReleaseWT/apps/docs/content/docs/release
FAIL
```

Restored, green again. Both halves recorded.

**Mutating the form the defect actually takes.** The guard's fixture is the real
shape: a `release-wt` branch carrying `board.json` under the Fumadocs prefix with
no `index.md` — the exact state of `2026-07-11-design-system` in the downstream
project where this was found.

**Scope parity.** The claim is "the release-ref probe is fixed", and the domain is
every site that resolves a release ref. `grep -rn '/index.md"'` over `cmd/` and
`internal/` found three such probes; all three now call the one helper. The fourth
hit (`internal/board/oracle.go:431`) is the legacy `index.md` *content* read inside
`ReadBoard`, not a ref probe, and is correctly left alone.

## Reachability artefact

The affordance is the CLI command that failed. Run against the real downstream repo
(`~/projects/fired`, release `2026-07-11-design-system`, board.json-native):

Before (installed `sworn` v0.1.0):

```text
sworn board: read board.json: git show HEAD:docs/release/2026-07-11-design-system/index.md:
fatal: path '...' does not exist in 'HEAD'
```

After (fixed binary):

```text
Release board: 2026-07-11-design-system

Track T1-foundation — in_progress
    DS01-token-foundation — unknown
    DS02-ci-enforcement — verified [verifier]
    DS03-visual-language — verified [implementer-2026-07-13-round8]
    DS04-alert-intent-consumer — planned
    ...
```

`sworn route DS04-alert-intent-consumer 2026-07-11-design-system` likewise goes from
the same HEAD error to a correct route (`state: planned` → `/implement-slice`),
confirming the loop's router path is fixed and not just the read command.

## Delivered

- `board.ReleaseRefFor` — single shared release-ref resolver, probing `board.json`
  first and `index.md` as a legacy fallback. Evidence: `internal/board/releaseref.go`.
- Fail-closed behaviour when a `release-wt` branch exists but carries no board
  record: an error naming that ref, replacing the silent HEAD retarget. Evidence:
  `TestReleaseRefFor_BranchWithoutRecordsFailsClosed`.
- Three duplicated probes (`cmd/sworn/board.go`, `cmd/sworn/route.go`,
  `internal/tui/board.go`) collapsed onto the helper. Evidence: `git diff --stat`,
  −33 lines.
- Regression guard with recorded mutation proof. Evidence: the Guard fidelity
  section above.

## Not delivered

- **The TUI degrades rather than fails closed.** On the no-board-record error
  `newSliceOracle` returns `nil`, falling back to the filesystem read.
  *Why:* the TUI is a read-only live view with a pre-existing, documented
  degrade-to-filesystem path (same function, for a repo with no git HEAD); taking
  the whole view down on a board skew is disproportionate. The gates
  (`sworn board`, `sworn route`) do fail closed on the same condition.
  *Tracking:* noted on sworn#100. *Acknowledgement:* in this bundle and in the
  session wrap-up.
- **`DS01-token-foundation — unknown`** appears in the now-readable board. Out of
  scope for this fix (the board reads correctly; that slice's `status.json` is a
  separate downstream question). *Tracking:* raised to the Coach in the wrap-up,
  not a sworn defect. *Acknowledgement:* session wrap-up.

## Divergence from plan

None. The plan was a single-call-site fix to `cmd/sworn/board.go`; the Rule 12 scope-parity
check found the same probe duplicated at two further sites, so the fix became one shared
helper across all three. That widening is a strengthening of the original scope, not a
departure from it, and is recorded here rather than silently absorbed.
