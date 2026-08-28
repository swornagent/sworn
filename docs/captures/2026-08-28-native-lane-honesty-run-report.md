# 2026-08-26-native-lane-honesty — run report (2026-08-28)

Seven slices ratified and authored 2026-08-26 (revision 1, target
20100e30), carried through **three plan revisions** to revision 3
(plan digest `sha256:c758ca5a…9a26f806`, target 086b8bbd), merged
2026-08-28 ~00:15 UTC at 326f485c. **7/7 verified, assembly passed.**

This was the first release authored on R4's own surface (`sworn plan
pin|lint|record` with digest-addressed contracts) — R4's acceptance
test in anger — and the first whose **final gate could only be passed
by its own delivered code**: assembly verification parked three times
on the pre-release engine and ran clean on a binary built from this
release's verified track head.

## Delivered

S1 CLI pin admission policy (#220 — pin becomes policy, live 2.1.241
shipped, executed-binary digest durable per dispatch, dead pin refuses
at admission) · S2 capture-proxy tool-less tolerance (#237) · S3
output stream economy (#241 — the 1MB cumulative cap becomes
manifest-governed economy with `ECONOMY_OUTPUT_BUDGET_EXCEEDED`) · S4
refusal taxonomy (typed `Kind` on `driver.ContractError`) · S5
preflight probes (#243) · S6 sanitization keeps diagnostics (#242) ·
S7 park lane-scoping (#239 as ruled).

S1-S3 land the three operator patches as product: no main-built ops
binary needs hand-patching for the claude lane any more.

## Run ledger (six runs, three plan revisions)

| run | what happened |
|---|---|
| r1 | S1 captain **escalated** (2aff93a2): A3's zero-burn refusal is unbuildable inside `internal/driver` — the only pre-attempt seam is `prepareDriverDispatch` in `internal/runtime`. Headless planner replan produced revision 2; operator approved the bytes |
| r2 | Revision-2 authority. The post-approval planner re-drive **could not recover its own approved bytes** and re-authored a contract whose A3 regressed to the impossible demand the escalate had retired. Operator carried the approved bytes byte-exact out of the sealed submission record. S1 design attempt 5 → captain PROCEED → build blocked: the implementer refused to fabricate the pin digest it could not read |
| r3 | Revision 3 (pin digests spelled in full as normative contract bytes). **S1, S2, S3, S4, S6 verified.** The implementer refused the digest again when offered through an operator answer it could not verify from the sandbox — correctly. S5 verified-then-failed on receipt quality; S7 blocked |
| r4 | S5 remediation deadlocked: `EMPTY_CANDIDATE` twice then transport failures, budget exhausted. Work identity is content-derived, so a fresh journal does not reset it |
| r5 | Operator reset the track ref to S5's captain-PROCEED and added operator patch #8 (submission content floor). **S5, S6, S7 verified — 7/7** |
| r6 | Run hosted on a binary built from the verified track head. **Assembly passed, merged** |

## What the release proved about the engine

- **Every blocker was an honesty mechanism firing correctly.** A
  captain refused to review an empty object; an implementer refused to
  fabricate a pin digest; the same implementer then refused an
  operator-supplied digest it could not verify from its sandbox; a
  verifier failed code it had independently confirmed correct because
  the receipt documented nothing.
- **Receipt ancestry adoption held across six journals** — S1-S4
  survived a track reset and three run generations without re-work.
- **Headless replan works** (R3-S7's machinery, first production use):
  the planner composed a full forward-only revision and yielded for
  approval without a human authoring bytes.
- **The release could not ship on the engine that preceded it.** The
  assembly failure was invisible on the old binary — `runner_error`
  with a zeroed observation, the exact black hole S6 fixes — and
  dispatched successfully on the new one.

## Defects found (filing with this report)

1. **The correction loop seals debugging probes as authoritative
   work.** A worker whose submission is rejected bisects the schema
   with a minimal payload; the loop accepts the first schema-valid
   result as the final answer. Six placeholder seals across four runs,
   one of them a **captain PROCEED** whose entire content was
   `summary: "probe: minimal submission to isolate field validation"`.
   The models label these honestly and the engine ignores the label.
   Operator patch #8 (content floor) reduced but did not end it — the
   next probe simply padded past the floor. The real fix is naming the
   refused field or bound back to the worker (S4 territory), not
   flooring the payload.
2. **No engine verb for "the code is right, re-seal the receipt."** A
   verifier FAIL on receipt quality creates an unfixable remediation
   loop when the implementation is already correct: the implementer
   has nothing to edit, every try dies `EMPTY_CANDIDATE`, and the run
   parks. This cost a track reset and ~734 lines of verified-good work.
3. **Operator answers reach workers as unattested recovery text.** A
   worker cannot distinguish an operator answer from injected data, so
   a well-designed one correctly refuses security-relevant values
   supplied that way. There is no operator channel for supplying a
   file to a sandbox. Values a worker must transcribe belong in
   contract bytes — truncated provenance prefixes are unusable
   downstream.
4. **`sworn driver certify` can no longer pass any native adapter.**
   S5 satisfied "stop claiming what you never evaluated" by removing
   the pass path entirely: `nativeCredentialLivenessCheck` already
   returns an `evaluated` flag and the call site discards it
   (`stale, _ :=`), so a credential that *was* read and *is* live
   still reports "unevaluated" — itself the false statement the
   contract set out to remove. Not a runtime blocker (the runtime
   never consults certification) but it breaks the documented
   pre-launch ritual and the fired-dogfood gate.
5. **A new test fixture asserted nothing.** S4's signalled-child
   acceptance (A4) was pinned by a fixture that self-sends SIGUSR1
   believing it fatal. In Go's signal table SIGUSR1 is notify-only, so
   an unwatched one is ignored: the fixture survived, slept into the
   test's own budget, and surfaced as `INVOCATION_TIMEOUT`. The
   acceptance criterion was never exercised.
6. **`AUTOMATION_BINDING_MISMATCH` on a recovery decision** whose
   binding the worker reports copying verbatim from the request.

## Convergence

Four CI rounds post-merge. Every red was this release's own new tests
meeting composition, and each was traced to a mechanism before being
repinned: the SIGUSR1 fixture defect (5 above — a real bug), the
needs-you action (S7 changed a park's next action from blind `retry`
to `review_park`), the parked-lane park cause (S7 gives the more
specific `identical_failure` row precedence; admissibility is
unaffected because the retry gate keys off a third try in
`OperationalFailed`, not the label), and a 5s fixture budget that
could no longer fit 200 in-process round trips under `-race` on a
loaded runner.

**CI green at f18ca4a6** (run 33139527109): every step ran and passed
— product tests, end-to-end, race, vet, darwin build, official binary.

One class is **not** this release's and is left un-hardened
deliberately: `PROCESS_START_FAILED` in the Bash tool sandbox hit 3 of
5 CI attempts on a *different* test each time
(`TestBashAcceptsCommandAliasExactlyOnce` twice, then
`TestToolBashNonZeroExitReturnsOutputAndExitCode`), never reproducible
locally, and R4 hit the same class on its own first attempt. A mask-fd
leak is ruled out — `tools_linux.go` closes them in a deferred loop.
The remaining candidates are the three raise sites in `runToolBash`
(`os.Pipe` under fd pressure, `bwrap` start, `/dev/null` open), and
pinning it down needs a CI-side reproduction rather than a speculative
fix. Filed with this report.

Worth recording for future gate-reading: CI steps stop at the first
failure, so a product-tests red **skips** end-to-end and race
entirely. A "fix pushed, CI red" round can leave later fixes entirely
unexercised — read the step list, not the run conclusion.
