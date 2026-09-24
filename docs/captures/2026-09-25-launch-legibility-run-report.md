# 2026-09-23-launch-legibility: run report

Release for ADR-0014 Track B release 2 (launch legibility). Six slices on one
track, driven by the Manager seat across five runs,
`2026-09-23-launch-legibility-r1` to `-r5`, on plan revision 1 (plan sha256
fae1106752fb, approved by the Principal 2026-09-23, prep commit 5f3257bb on
main 6ea55d02).

## Outcome

Merged. All six slices verified PASS by the independent Verifier on first
review of their sealed candidate; the assembly verification passed in r5 and
protocol.merge moved the release branch to b1f59791. The release closes
#324, #325, #326, #327, #330, #331, #336, #340 and #342, and the advice-text
half of #349.

| Slice | Implementer | Lead design reviews | Candidates / tries | Verifier |
|---|---|---|---|---|
| S1-launch-refusals | muse-spark-1.3 | proceed | 1 / 6 | PASS 420e0a5e |
| S2-canonical-authoring | muse-spark-1.3 | proceed | 1 / 2 | PASS 2c3065d1 |
| S3-host-check-failure-facts | muse-spark-1.3 | proceed | 1 / 3 | PASS 9db0a0ff |
| S4-lane-live-probe | muse-spark-1.3 (design), claude-sonnet-5 (implementation) | proceed | 1 / 4 | PASS ef51af47 |
| S5-transient-provider-backoff | claude-sonnet-5 | revise, proceed | 1 / 6 | PASS afd60e96 |
| S6-context-window-clamp | claude-sonnet-5 | revise, revise, proceed | 1 / 1 | PASS b1f59791 |
| Assembly | (engine) | | 1 | PASS 002f1b8c, merge 4c5c5062 |

Planner, Lead and Verifier were claude-opus-5-5 throughout. Of 22
implementation tries, 16 failed; six of those failures were findings about
the candidate (five host-check failures on real defects, and one anchor-gate
refusal). The other ten were environment and engine faults listed below.

## Runs

| Run | Window (UTC) | Why it ended | Dispatches | Host checks |
|---|---|---|---|---|
| r1 | 09-23 11:35 to 13:21 | parked: the pinned Claude Code 2.1.241 cannot call claude-opus-5-5; cancelled after #353 | 4 | 0 |
| r2 | 13:21 to 19:54 | parked: Claude Code 2.1.280's init event advertised two new capabilities; cancelled after #354 | 3 | 0 |
| r3 | 19:54 to 09-24 03:04 | parked: host checks exit 127, `go` not on the systemd user unit's PATH; a same-run retry replayed the stored failure; cancelled | 6 | 2 |
| r4 | 03:04 to 11:43 | S1 to S3 verified; paused and cancelled for the roster change off the Meta lane | 17 | 46 |
| r5 | 11:43 to 22:43 | S4 to S6 verified, assembly PASS, merged | 23 | 33 |

## What was delivered

- S1: `sworn serve` prints the typed code and the failing input; the
  operator config loader returns typed refusals (including the 0600 mode
  requirement); plan record, pin and lint accept abbreviated object ids
  resolved through one sanitized gitx resolver; a new `docs/launch.md`.
- S2: `sworn manifest canonical` for runtime manifests and driver configs,
  judged by the same admission functions the engine uses; `plan pin
  --write`; refusal text that names the fixing command.
- S3: a bounded host check failure fact (command, exit code, rerun, not-run
  checks, output excerpt) in the shared status projection; check outcomes
  beside effect states; the assembly BLOCKED park advice names manager
  policy M9.
- S4: `sworn driver probe`, one minimal live request per named lane,
  exposed to the runtime as one function; `doctor --all` and `certify --all`
  name the missing production families from one roster declaration.
- S5: after a transient provider failure the engine waits with a bounded,
  journaled backoff and starts the next try only after a live probe passes,
  parking with a typed provider-stall cause after 30 minutes.
- S6: an optional `context_window_tokens` on HTTP profiles, a clamp of
  max_output_tokens from the previous turn's reported usage, a typed economy
  code when a turn cannot fit, and input tokens in the dispatch view.

## Engine and policy changes landed on main during the release

Each found by the run and merged by the Principal before the run could
finish or the next run could start:

- #353: native CLI admission accepts any well-formed version; the compiled
  version is reported as tested or untested (`cli_compatibility`), never
  enforced. The configured digest and version output still bind the binary
  at every launch.
- #354: the Claude init check admits the two capabilities Claude Code
  2.1.280 advertises; unknown capabilities still fail closed.
- #360: the native tool broker queues concurrent tool calls instead of
  refusing them (#359, part).
- #352: the Manager skill keeps run journals in the ops home and archives
  them at end of release (#351).
- #356: manager policy version 3, entry M10 (a host check that cannot find
  its command: fix the serve unit's PATH, relaunch on a fresh journal).

## Verification of the promotion head

The promotion result, the release head b1f59791 merged with main eaeb0b5d
(which carries #352, #353, #354, #356 and #360), was checked out in a fresh
worktree and verified on this host one suite at a time: cmd/sworn, all nine
internal packages and both tools packages ok; the serial end-to-end suite ok
in 2939 s; the race suite ok on every product package (internal/runtime
660 s); vet, gofmt, module tidiness, `git diff --check` and the
darwin/arm64 build clean. The merge had no conflicts. No flake reruns were
needed.

## Seat observations

- Every park in this release was an environment or engine gap before it was
  a candidate finding, and each was a check that was true at the moment it
  ran but not for the life of the work: a pinned CLI that could not call the
  approved model, a CLI whose event surface had moved, a PATH that an older
  systemd user session had carried, an OAuth token that expired mid-dispatch
  (#358), and a tool broker that spent its call budget on refused polls
  (#359). Each is filed; three are already fixed on main.
- The failure turn context from the previous release paid off directly: the
  OAuth expiry was diagnosed from the status projection alone ("Failed to
  authenticate: OAuth session expired").
- Other diagnoses still needed a delegated journal read, each a legibility
  gap now filed: an exit-127 host check replayed as a stored result (#355);
  a pause that turned a passing candidate into JOURNAL_WRITE_FAILED (#357);
  `error_max_turns` reported as PROVIDER_TRANSPORT_FAILED (#359); a seal
  refusal lost when a retry starts a new epoch, and `anchor_substitutes`
  never offered to the implementer (#361).
- The roster moved off the Meta lane at r4 for cost, on the Principal's
  decision. The engine made the switch cheap: one manifest field and a new
  run id; every receipt carried.
- Seat errors, recorded in the decision journal: the pre-launch live check
  used the host `claude` rather than the engine's pinned copy, so it could
  not catch r1's failure; a pause issued during host checks cost S4 a
  passing candidate (#357); a fresh-epoch retry of an anchor refusal lost
  the refusal on its first try (#361); a binary copy over a running serve
  unit failed "Text file busy" and the chained cancel did not run until
  redone.
- Delegation: the Principal delegated operational recovery (environment
  fixes, pause, retry, cancel and relaunch with only the run id changed) to
  the seat during r3. Plan, roster and merge decisions stayed Type-1: plan
  approval, the Opus 5.5 roster, the move off Meta, and every merge to main
  (#352, #353, #354, #356, #360).
- Journals r1 to r5 are archived in the ops home with SHA-256 sums; none
  lived in the release worktree.

## Issues filed from this release

#355, #357, #358, #359, #361 (#359 partly closed by #360). Rulings recorded
on #349 (manager policy M9 stays the route) and #319 (decide capability
design notes).
