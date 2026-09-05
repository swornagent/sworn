# 2026-09-03-foreign-repo-honesty - run report

**Outcome: MERGED.** Six slices, all verified PASS, assembly verdict passed
and composed at `f583b8fc` on `release/2026-09-03-foreign-repo-honesty` on
2026-09-05 08:09Z by the engine itself (`baton.merge`); record ref
`release-wt/2026-09-03-foreign-repo-honesty` at `93b2688a`. Three plan
revisions (rev 3 authored in-run by the planner after a captain escalation),
eight run journals (r1-r5 walled by engine defects, r6-r8 delivered), three
engine fixes merged to main mid-release.

Theme delivered: the engine stops assuming it is operating on itself. The
release was sliced from the defects the first foreign-repository release
(fired, 2026-09-02-engine-idempotence) surfaced, and it found three more
engine defects of the same family while being driven.

## Outcome

| slice | outcome | tries (impl) | notes |
|---|---|---|---|
| S1-refusals-carry-cause | pass | 2 | r6; t1 lost to a continuation fallback on the gemini lane |
| S2-foreign-layouts | pass | 2 | captain escalated the rev-2 contract; delivered on rev 3 |
| S3-bootstrap-replan-honesty | pass | 4 | r7 t1 parked on the 200-turn budget (#288); r8 t1-t2 PROVIDER_LIMITED; t3 sealed |
| S4-sandbox-start-evidence | pass | 1 | |
| S5-codex-tool-budget-parity | pass | 2 | t1 failed its own host check; t2 sealed |
| S6-double-park-honesty | pass | 2 | t1 failed its own host check; captain t1 PROVIDER_TRANSPORT_FAILED |

Release diff against main: 70 files, +6573 / -625, 45 commits.
Closes #254 #257 #259 #263 #272 #273 #278. Not delivered: #255, #270.

## Roster

planner + recovery: qwencloud/qwen3.8-max; implementer (design + implementation):
google-flash/gemini-3.8-flash; captain: claude/claude-opus-5; verifier and
assembly verification: claude/claude-sonnet-5. Host checks: the product suite
and the race suite on the host (declared `host_checks` from rev 2 onward).

## Runs

| run | binary | what happened |
|---|---|---|
| r1 | 42481a5d | S1 design; captain admission NATIVE_PIN_DEAD (resolv.conf drift, #279) |
| r2 | 42481a5d | S1 implemented and sealed; verifier hung in-sandbox (no C toolchain) -> rev 2 moved go test to host_checks |
| r3 | 42481a5d | host check false-failed: `nohup` launch inherited SIGHUP-ignore into a test fixture (#280); gemini PROVIDER_LIMITED |
| r4, r5 | 42481a5d | host checks passed, seal walled CORRUPT_JOURNAL: **#281** (host manifest vs dispatch proof) |
| r6 | 3e1b4b9e (#282 #284) | S1 sealed and verified; S2 escalated by the captain; planner rev 3; planner t1 lost to **#285** (scratch cleanup); t2 yielded, answered, sealed rev 3 |
| r7 | 5a9aefec (#286) | S2 (rev 3) sealed and verified; S3 designed; S3 implementation parked at 200 turns (**#288**) |
| r8 | 5a9aefec + max_turns_per_work 800 | S3-S6 end to end, assembly verdict, merge |

## Effort (journaled tool-result turns, r6-r8)

| slice | design | captain | implementation | verify |
|---|---|---|---|---|
| S1 | (adopted from r2) | (adopted) | 333 | 36 calls / 1 turn |
| S2 | 94 (2 tries) | 70 calls / 2 turns | 164 (2 tries) | 34 / 1 |
| S3 | 55 | 35 / 1 | 823 (4 tries) | 50 / 1 |
| S4 | 78 | 41 / 1 | 115 | 28 / 1 |
| S5 | 27 | 11 / 1 | 184 (2 tries) | 29 / 1 |
| S6 | 41 | 53 / 2 | 269 (2 tries) | 31 / 1 |
| release planner (rev 3) | 190 turns / 235 calls (2 tries) | | | assembly 29 / 1 |

Total across r6-r8: 2387 provider turns, 2865 tool calls. On the gemini
lane every tool call is its own turn; the claude lanes batch 30-70 calls into
one or two turns.

Of those 2387 turns, 1184 (half) were spent on tries that a later try
superseded. Attributed from the journals:

| cause | turns | share |
|---|---|---|
| engine or provider faults: S1 t1 continuation fallback (158), planner t1 #285 (59), S2 impl t1 CONTINUATION_INVALID (50), S3 r7 t1 200-turn park #288 (200), S3 r8 t1+t2 PROVIDER_LIMITED (418) | 885 | 37% |
| the process working as designed: S2 redesign after the captain's escalation (66), S5 and S6 first candidates caught by host checks (232), one captain transport retry (1) | 299 | 13% |

Roughly a third of the release's model spend went to faults the engine or the
provider lane introduced, not to the work.

## Engine defects found and fixed during the release

| issue | class | fix |
|---|---|---|
| #279 | native admission pins volatile runtime files (resolv.conf) | re-pinned; open |
| #280 | `nohup` SIGHUP-ignore inherited into host-check fixtures | operator rule (setsid); open |
| **#281** | implementer host_checks manifest fails the dispatch proof -> CORRUPT_JOURNAL, candidate stranded | **#282** merged |
| **#277** | hosted-runner sandbox flake = child reaped before the handshake probe (ESRCH read as "dead child") | **#284** merged |
| **#285** | read-only subtree in invocation scratch -> RemoveAll fails -> ADAPTER_FAILURE, yield discarded | **#286** merged |
| #283 | engine commit subjects fail conventional-commit PR gates (from fired PR #1734) | open |
| #287 | re-approval of a resealed proposal with unchanged substance is coach-approvable | open (design) |
| #288 | 200-turn default binds at ~1/3 of an honest implementation on one-call-per-turn lanes | open |

Three of these (#281, #277, #285) share one shape: a dispatch or refusal
record that carried no cause, so each took an hour of forensics to name a
defect that a single recorded sentence would have named in a minute. S1 of
this release is the operator-surface half of that fix; the dispatch-record
half is the obvious next slice.

## Operator observations

- Human round trips were the slowest step: parks sat 20-80 minutes before the
  operator noticed. The r8 watch reported the exit within a minute.
- Two of three human questions carried no information the engine could not
  have checked itself (#287).
- The captain earned its keep: the S2 escalation was measured (94 + 104
  failing tests) and the resulting revision 3 was the right call.
- The verifier passed every candidate it saw first time; the host checks
  caught two candidates (S5, S6) the implementer thought were done.

## Verification of the merged head

On `f583b8fc` in a detached worktree, host toolchain, `-buildvcs=false`:

| check | result |
|---|---|
| `go test -count=1 -timeout 40m ./...` | 13 packages ok, exit 0; `test/e2e` ok in 1190s |
| `gofmt -l ./cmd ./internal ./tools` | clean |
| `go vet ./...` | clean |
| merge dry run against `origin/main` (6 commits ahead of the release) | clean, no conflicts |
| private-term grep over the release diff's added lines | 0 hits |

Every slice's candidate had already passed both declared host checks (the
product suite and the race suite) at its seal, and the assembly verifier
passed the composed tree; this is the operator's independent repeat on the
exact merged head before promotion. CI on the promotion PR is the third
witness.
