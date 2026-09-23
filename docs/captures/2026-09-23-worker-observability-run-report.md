# 2026-09-22-worker-observability: run report

Release for sworn#294, the first release of ADR-0014 Track B (legibility).
Three slices on one track, driven by the Manager seat across seven runs,
`2026-09-22-worker-observability-r1` to `-r7`, on main a4a3e6f9 (plan
revision 1), then revisions 2 and 3 with byte-identical contracts.

## Outcome

Merged. All three slices verified PASS by the independent Verifier; the
assembly verification passed on revision 3 in r7, on evidence the engine
produced by executing all eight declared host checks on the assembled
candidate in the judging run; protocol.merge into the release branch. No
hand operator work on any run: every action went through the sworn CLI
under a journaled policy entry or a Principal decision.

| Slice | Implementer | Candidates | Verifier | Receipt |
|---|---|---|---|---|
| S1-native-turn-journal | claude-sonnet-5 (r1) | 3 | FAIL then PASS | a819e4f0 |
| S2-live-worker-stream | muse-spark-1.3 at effort max (r3) | 2 | PASS first attempt | c0e0aa24 |
| S3-failure-turn-context | muse-spark-1.3 at effort max (r3) | 1 (2 tries) | PASS first attempt | 113d0929 |
| Assembly | (engine) | 1 | BLOCKED x2, then PASS | 7c0c77c7 |

Lead and Verifier were claude-opus-5 throughout. Every design, including
all three from the Muse lane, passed the Lead on first review.

## Runs

| Run | Window (UTC) | Why it ended | Dispatches | Host checks |
|---|---|---|---|---|
| r1 | 09-21 20:38 to 00:12 | cancelled at the S1 boundary for the roster change the Principal approved | 8 | 22 |
| r2 | 00:12 to 02:39 | parked twice on provider admission stalls; cancelled for an adapter change | 9 | 0 |
| r3 | 02:39 to 09:07 | S2 and S3 verified; assembly BLOCKED (no host evidence projected) | 9 | 23 |
| r4 | 09:07 to 09:13 | adopted the BLOCKED record; parked with no dispatch | 0 | 0 |
| r5 | 09:14 to 12:17 | revision 2; assembly BLOCKED (evidence unprovable across runs) | 1 | 0 |
| r6 | 12:17 to 21:10 | adopted the second BLOCKED record; parked with no dispatch | 0 | 0 |
| r7 | 21:11 to 22:18 | revision 3; eight host checks on the assembled candidate, PASS, merged | 1 | 8 |

## What was delivered

- S1: the native reader parses worker-turn events on the Claude and Codex
  CLI lanes, a separately named observation turn keys tool results to the
  turn they belong to, and one bounded, redacted `worker_turn_observed`
  event (`sworn.worker-turn/v1`) is journaled per turn through the existing
  observation mechanism. Known, accepted limitation: attribution of a tool
  result reads the turn counter at call time across two unordered channels
  (design 4219adb4, Lead correction 5); the deterministic fix is #337.
- S2: one activity projection over journaled worker turns and tool results,
  a content-bearing SSE route beside the byte-unchanged invalidation route,
  an in-memory ring fed after the durable append (so the live stream and the
  journal page agree by offset), an activity pane on the browser board and
  an activity screen in the TUI.
- S3: a bounded tail of the worker's last turns on every operational
  failure record, in the same write as the failure, shown in the status
  projection.

## Engine changes landed on main during the release

Each found by the run and merged before the run could finish: #335
(credential newline trim, pre-dial rejection code, per-adapter output token
ceiling), #339 (optional reasoning summary streaming on the Responses
surface), #344 and #348 (assembly verification receives host-boundary
evidence, then executes the declared host checks on the assembled candidate
so assembly evidence is produced by the judging run, reused only on an
identical same-run product tree).

## Verification of the merged head

Release head a68eaeea checked out in a fresh worktree; one package at a time
on this host: cmd/sworn, all nine internal packages and both tools packages
ok; the serial end-to-end suite ok in 2634 s; the race suite ok on
internal/runtime, internal/driver and internal/cockpit; vet, gofmt, module
tidiness and the darwin/arm64 build clean. No flake reruns were needed.

## Seat observations

- The provider lane cost more time than the models. Meta's API stalled
  admission in three windows of a few minutes; the engine's immediate
  retries spent the identical-failure budget inside one window (#340).
  Effort max with streamed reasoning summaries ran about 700 Muse turns in
  r3 with no stall. One try was lost to a request whose input plus output
  ceiling exceeded the model window (#342).
- The assembly gap was found only because a strict Verifier refused to PASS
  on evidence it could not see. Two earlier releases passed that step on
  leniency. Fixing it took two engine changes and two byte-identical plan
  revisions, because a BLOCKED assembly record poisons its revision and the
  protocol's only exit is a new one (#349).
- Legibility gaps the seat had to work around, each filed: the status
  projection names no check, exit code or excerpt for HOST_CHECK_FAILED
  (#336); provider failures arrive with the code only; doctor makes no live
  call (#331). Seat errors, recorded in the decision journal: calling an
  accepted design limitation a masked defect before reading the Lead
  receipt; cancelling r3 before retrying its assembly under the fixed
  binary; two monitor scripts that named a non-existent binary.
- A second harness (Muse Code, on a non-Anthropic model) filled the Manager
  seat read-only over the MCP surface alone and reached the same Type-1
  decision from the same projection (evidence on #345).

## Issues filed from this release

#328 to #343, #347, #349 (#328, #329, #338, #343 closed by the merges above).
