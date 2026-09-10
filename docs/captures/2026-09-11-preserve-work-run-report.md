# 2026-09-05-preserve-work - run report

**Outcome: MERGED.** Four slices, all verified PASS, assembly verdict passed
and composed at `efbcc191` on `release/2026-09-05-preserve-work` on
2026-09-10 19:32Z by the engine itself (`baton.merge`). Four plan revisions
(rev 2 and rev 3 recovered a walled S1; rev 4 changed the implementer lane
and gave S2 operator context), five run journals (r1-r3 and `recovery` on
gemini-3.8-flash, `rev4` on claude-sonnet-5 delivered S2-S4), zero operator
interventions on the delivering run after launch.

Theme delivered: the engine preserves work it has not yet verified. Durable
unverified checkpoints (S1), reconciliation of interrupted implementation
work before cleanup (S2), targeted repair of submission and evidence defects
against retained work (S3), and resumable budget stops with an explicit,
bounded operator grant (S4). S1's checkpoints kept every one of the
delivering run's 16 candidates recoverable across its own failures.

## Outcome

| slice | outcome | design tries | impl attempts / candidates | notes |
|---|---|---|---|---|
| S1-durable-unverified-checkpoints | pass | 1 | 1 | verified under rev 3 on the `recovery` run; carried by digest into rev 4 |
| S2-interrupted-work-reconciliation | pass | 1 | 3 attempts, 4 candidates | t1 lost the race gate to a flaky fixture (#295); the unchanged resubmission replayed the recorded fail (#296); FAIL x2 for missing crash-cut e2e anchors; PASS on attempt 3 |
| S3-targeted-handoff-repair | pass | 2 (one REVISE) | 2 attempts, 3 candidates | t1 broke three runtime tests, repaired from bound host evidence; FAIL once for test-only gaps; PASS on attempt 2 |
| S4-resumable-budget-stops | pass | 3 (a degenerate submission, #300, then a substantive REVISE) | 11 attempts, 12 candidates | mechanism accepted by attempt 3; attempts 4-11 each added one chunk of the real-adapter / real-binary evidence tier; PASS on attempt 11 |

Release diff against main: 114 files, +14200 / -289, 88 commits. Includes the
operator-validated host-check repair-context fix (`3a0f20fe`, admitted as S1
recovery input under rev 3). Refs #288, #291, #292. Closes nothing by
number: the release was planned from a capability, not an issue list.

## Roster

rev 1-3 and `recovery`: planner + recovery qwencloud/qwen3.8-max; implementer
google-flash / google-native gemini-3.8-flash; captain claude/claude-opus-5;
verifier claude/claude-sonnet-5. rev 4 (`rev4` run): implementer
claude/claude-sonnet-5 (Claude Code CLI lane, batching tool calls), captain,
verifier and assembly verification claude/claude-opus-5, per-invocation
timeout 2 h. Host checks: product suite, 45-minute serial e2e, 20-minute race
suite, vet, gofmt, tidy, diff check, Darwin build, all declared `host_checks`.

## Runs

| run | lane | what happened |
|---|---|---|
| r1 (2026-09-05) | gemini OpenAI-compat | S1 implementation lost to PROVIDER_LIMITED (3M input tokens/min tier cap); no seal |
| r2 (2026-09-06) | gemini native, 2.4M/min pacing | S1 candidate 2fcaa718 violated A4 (scope refusal deleted progress); paused after the e2e fixture failed deterministically |
| r3 (2026-09-06) | gemini native, rev 2 | three S1 tries each failed host checks without repair context; parked; codex-era investigation found two fixture defects and the missing repair binding |
| recovery (2026-09-08) | gemini native, rev 3, repaired binary | S1 verified PASS; S2 t1 lost to a Gemini correlate defect (#291) then a scope fence on a stray build artefact; three retries each spent the 60-minute invocation cap cold (#292); parked on exhaustion |
| rev4 (2026-09-09 21:32 to 2026-09-10 19:32Z) | claude-sonnet-5 | S2, S3, S4 designed, implemented, verified; assembly PASS; merged. 494 events, 16 sealed candidates, 133 host checks (130 pass, 3 fail), 16 verifications, 6 designs, 6 captain reviews, 24 in-dispatch turn recoveries, 1 transport failure |

## What the run taught the engine (filed)

- #291 the Gemini correlate failure kept no cause in the dispatch record (fix: PR #297).
- #292 every retry starts a fresh conversation; implementer and verifier should each resume their own session (ruled).
- #293 a target-ref move erases an exhaustion park and re-arms the try budget (fix: PR #299, held for this promotion).
- #294 native CLI lanes journal one row per dispatch instead of one per turn (ruled).
- #295 a one-second wait in a webhook test flakes under the race suite; repaired in-slice by the implementer.
- #296 an unchanged candidate replays a recorded host-check failure instead of re-running it.
- #298 the TUI drive PTY tests fail inside sandboxed workers on nested containment (fix: PR #298).
- #300 a degenerate repeated-token submission passes the byte-length floor.
- #301 seal-time anchor-presence gate: six rounds were spent learning a named file was untouched.
- #302 phased evidence: a verifier pre-verdict after the product suite; eleven of fourteen rounds ended in a verdict decidable without the e2e and race suites (ruled as the next release).

## Cost shape

Every candidate paid the full declared sequence before the verifier saw it:
about 8 min product suite, 35 min serial e2e, 12 min race and quick checks,
then 5 to 27 min of Opus verification. Implementation on the Sonnet lane took
7 to 75 min per attempt. Fourteen full rounds in 22 h of wall clock, of which
three ended in a PASS.
