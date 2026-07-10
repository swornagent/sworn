# Captain review — S10-conformance-sit
Date: 2026-07-11
Design commit: 44d87646e45b2384ee3154b31e2386225c1951c1

## Pins

1. [escalate] §2.D2 — AC-01's undeclared-role clause contradicts the landed ADR-0012 contract; the clause as designed fails all four real drivers.
   What I observed: D2 plans to "call Dispatch with Role: 'captain' against a driver whose RoleSet excludes it" and assert "declared-incapable roles error, never panic". Live code says otherwise: `internal/driver/driver.go` (DispatchInput.Role doc) states "Resolution must have already confirmed the target driver's Roles().Has(Role) before calling Dispatch — Dispatch itself does not re-check it", and none of the four drivers guards role membership in Dispatch (claude.go Dispatch has no role check — an undeclared captain dispatch builds a prompt and spawns; codex.go same; inprocess.go:108-140 same). Against a fake binary exiting 0, an undeclared-role dispatch returns StatusOK, so the conformance clause FAILs claude and codex as the contract stands. The defect is in the seam between spec AC-01 ("a dispatch with a Role outside the driver's declared RoleSet returns an error") and the S01-verified contract that deliberately places role enforcement at resolution (registry.Resolve role-arm, registry.go:157-160), not at Dispatch.
   What to ask the implementer: nothing — this is not the implementer's call. The Coach must pick: (a) amend AC-01's clause (via /replan-release) so the suite asserts role rejection at the landed enforcement boundary (a registry-wrapped Resolve returns the role-arm error for an undeclared role; Dispatch is additionally asserted not to panic), or (b) harden all four drivers' Dispatch with a fail-closed Roles().Has(in.Role) guard — a production change to the S01 Type-1 contract posture, and outside this slice's pure-test scope as designed. Option (a) preserves the landed Type-1 decision; option (b) buys defense-in-depth at the cost of amending ADR-0012's stated posture. The Coach picks.

2. [mechanical] §2.D5 — CRITICAL: the SIT fixture inventory omits everything the real RunSlice path gates on before implement; as designed, TestLoopSIT dies at the Definition-of-Ready gate, not at verification.
   What I observed: D5's fixture is "board.json, one track, one slice (spec.json + status.json at planned)". But wiring the REAL run.RunSlice (D5's whole point) pulls in: (i) the design-TL;DR captain dispatch (slice.go:302-349) and captain review (slice.go:351-441) — the stub must serve RoleCaptain with scripted output that captain.Review parses to zero escalate pins, or the run halts; (ii) implement.Run's design_review→in_progress TransitionGate runs CheckDoR (implement.go:60-80, ready.go:73-133): rtm.Build hard-errors when `intake.md` is absent from the fixture releaseDir (rtm.go:96-99), reqverify dispatches Role=RoleCaptain on the implementer driver (ready.go:34-35) and needs scripted per-AC PASS text, and reqvalidate.Run needs validation-ratification records; (iii) proof.md is written by implement.Run itself from git state (so the stub need not touch disk — confirmed, no pin there).
   What to ask the implementer: extend the fixture to be DoR-complete — intake.md with the need id, spec.json covers_needs tracing to it, reqvalidate ratification records, index.md if rtm.Build reads it — and script the StubDriver's captain-leg outputs (design TL;DR, review with no escalate pins, reqverify PASS text) in addition to the implement/verify legs D4 already covers. Update design.md §Files-touched fixture row accordingly while implementing.

3. [mechanical] §2/§Approach — CRITICAL: the cited in-process httptest wiring does not exist; `model.ProviderConfig` has no base-URL field and the only injection seam is unexported.
   What I observed: the design says callers "close over their fake-binary path or `httptest` URL" via `inprocess.NewOAIChat(cfg)`. ProviderConfig (model/provider.go:10-33) carries keys/hosts only — no OpenAI-family base-URL override. The in-package tests inject via the unexported `InProcess.newClient` field (inprocess_test.go testDriver, inprocess.go:72-74), which `internal/driver/drivertest` and `internal/driver/conformance_all_test.go` cannot reach. Subprocess wiring is fine: ClaudeDriver.Binary / CodexDriver.Binary are exported (claude.go:44, codex.go:32).
   What to ask the implementer: pick whichever of the two determinable routes works — (a) proxy route with zero production edits: fake credentials + SWORN_PROXY_URL (t.Setenv auto-restores, Rule 11-clean) so model.ResolveLoopClient (config.go:192-204) routes the dispatch to the httptest server via proxyClient; verify account.Load/Endpoint can be satisfied hermetically; or (b) add a minimal exported client-factory seam to internal/driver/inprocess — a production edit that must then be declared as a touchpoint addition and a files-touched divergence in proof.md (it contradicts the design's "nothing existing edited" line). Prefer (a) if it works; either way record the choice in design.md/proof.

4. [mechanical] §Approach — `Registry.Drivers()` returns `[]Info` (name/prefixes/roles), not driver instances; "auto-enrolled with zero suite edits" overstates what the enumeration can deliver.
   What I observed: registry.go:170-186 — Info carries Name/Prefixes/Roles only. The suite cannot get a `driver.Driver` (let alone a fresh-instance factory per D1) from Drivers(); Resolve returns the shared singleton, which breaks D1's fresh-driver-per-subtest requirement and offers no fake-transport hook. A future fifth driver's fake wiring (its fake binary or fake server) can never be auto-derived.
   What to ask the implementer: implement enrolment as fail-closed detection — iterate Drivers() for the registered name list, look each name up in a test-owned name→factory map (factories constructing fresh, fake-wired drivers), and fail the test loudly on any registered name missing from the map. That preserves AC-02's intent (a newly registered driver cannot ship unenrolled silently) with an honest mechanism. State it in design.md while implementing.

5. [mechanical] §2.D2 — the `RequiresWorktree bool` escape hatch is dead configuration at birth, and two citation drifts need fixing.
   What I observed: all four registered drivers already enforce the Rule-11 guard before any work — claude.go Dispatch calls AssertWorktree first, codex.go same, inprocess.go:112-117 rejects empty WorktreeRoot then calls AssertWorktree. No registered driver would set RequiresWorktree=false, and per the design's own R-02 posture, a clause a driver can't run is "a contract-doc defect surfaced to the human", not a silent opt-out knob. Also: (a) the Result field is `DurationMS int64` (driver.go), not `Duration` as D2/AC-01 phrasing implies; (b) §Approach gives `Run(t, newDriver)` while the files table gives `Run(t, newDriver, opts)` — pick one signature.
   What to ask the implementer: default the worktree clause to mandatory for every driver; if an exemption knob is kept for future transport-less drivers, default it to required and document that setting it is the R-02 surface-to-human path. Fix the DurationMS naming and the Run signature inconsistency inline.

6. [memory-cited] §2.D5/D6 — the SIT's fixture diff passes through the mock-lint and first-pass gates; scripted prose can false-positive the boundary_mock scanner.
   What I observed: RunSlice runs gate.RunMock (slice.go:705-721) and verify.RunFirstPass (slice.go:722-745, including undeclared-boundary-mock checks) over the fixture slice's committed diff — which will contain the stub's scripted design.md/review.md prose. The scanner false-positives on prose (sworn#87), and the SIT's scripted text will naturally want words like "stub" and "mock".
   What to ask the implementer: keep the StubDriver's scripted artefact text free of scanner trigger phrasing, or pre-declare deferrals in the fixture status.json open_deferrals. Also confirm verify.RunFirstPass accepts the fixture's spec.json as SpecPath — do not manufacture a spec.md in the fixture to appease a gate that doesn't need it.
   Citation: [[feedback_releaseverify_specmd_false_fail]] (spec-v1 slices have no spec.md; boundary_mock scanner false-positives on prose — declare, don't contort).

7. [mechanical] §2.D7 — confirmed: AC-03's boundary is "verified", not "merged"; D7 stands as designed.
   What I observed: the implementer asked the reviewer to confirm the scope reading. AC-03's text says "at least one slice reaches verified" verbatim, and ParallelOptions.MergeTrackFn's doc confirms nil ⇒ auto-skip (parallel.go:69-73). The answer is in the spec; no Coach decision needed.
   What to ask the implementer: nothing — proceed with MergeTrackFn nil as designed.

8. [memory-cited] §2.D4 — shared StubDriver (one type, two consumers) acknowledged; the sharing rationale is exactly the anti-leaf-mocking lesson this slice exists to encode.
   What I observed: D4 keeps the conformance-certified stub and the SIT-dispatched stub as one implementation so they cannot silently diverge. That matches the eval's failure mode — DOA on unit-green because every parallel_test.go case injects fakeRunSlicePass (confirmed at parallel_test.go:20-21) and nothing booted the assembled loop.
   What to ask the implementer: acknowledged as designed; note the stub must also script captain-leg and reqverify outputs (pin 2), so give it the per-role queue/callback shape D4 already proposes rather than a single canned Result.
   Citation: [[project_parallel_cold_start_broken]] (engine-not-model bottleneck; loop reached verified for ZERO slices on unit-green code).

## Summary

Pins: 8 total — 5 [mechanical], 2 [memory-cited], 1 [escalate]
Critical pins: 1, 2, 3 (1 = spec/contract conflict that fails all four drivers as written; 2 = SIT dies at the DoR gate as designed; 3 = cited in-process wiring mechanism does not exist)

## Smaller flags (not pins, worth one-line acknowledgement)

- status.json carries no `design_decisions` field — record the D1/D4/D7 Type-2 classifications there when transitioning, so the design-fit gate has something to read.
- Pre-existing fused-comment corruption residue in three landed files (parallel.go:81, parallel_test.go:20, model/anthropic_test.go:253) — newline-eating pattern, cosmetic only, filed as swornagent/sworn#91; out of this slice's scope.
- The SIT must use stub-prefixed model IDs for ImplementerModel AND VerifierModel so every leg (captain/design/DoR/implement/verify) resolves to the stub; subprocess drivers do not declare RoleCaptain (sworn#86), which is why the captain leg fail-opens in production — the stub declaring all three roles keeps the SIT's Rule-9 legs real.
- D6's DB-dump citation checks out: TestRunParallel_Basic's tracks/events DDL is at parallel_test.go:113-114.
- Design.md's other citations verified against live code: cmd/sworn/run.go:172-173 runSliceFn closure, parallel.go:196-207 cold-start bootstrap ("branch %s absent — creating it from HEAD"), board.NewOracleReaderAdapterFromRepo (oracle.go:691-695), RunSliceOptions.Registry (slice.go:60-67), registry.Default's four drivers + openai-responses alias (registry.go:276-306).

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR strong design on the right architecture (real RunParallel/RunSlice, shared stub, hermetic git fixture) — but three load-bearing gaps to close inline. 8 pins + 4 flags:

1. **AC-01 undeclared-role clause.** Per the Coach's resolution of the spec/contract conflict: implement the role-rejection clause at the boundary the Coach designates (resolution-boundary assertion per the landed ADR-0012 posture, or a Dispatch-level guard if the Coach amends the contract). Additionally always assert no-panic on an undeclared-role dispatch.
2. **Make the SIT fixture DoR-complete.** The real RunSlice path runs design TL;DR + captain review + CheckDoR before implement. Add intake.md (need ids), covers_needs tracing, reqvalidate ratification records, and any index.md rtm.Build reads; script the StubDriver's captain-leg outputs (design TL;DR text, review with zero escalate pins, reqverify per-AC PASS text). proof.md is written by implement.Run itself — the stub need not touch disk.
3. **In-process httptest wiring.** ProviderConfig has no base-URL field and InProcess.newClient is unexported. Use the proxy route (fake credentials + SWORN_PROXY_URL via t.Setenv → ResolveLoopClient → httptest) if it can be satisfied hermetically; otherwise add a minimal exported client-factory seam to internal/driver/inprocess and declare the touchpoint addition as a divergence in proof. Record the choice in design.md.
4. **Enrolment is fail-closed detection, not zero-edit.** Drivers() returns []Info, not instances. Iterate Drivers() for names, map name→fresh-fake-wired factory, and fail the test on any registered name missing from the map.
5. **Drop or invert the RequiresWorktree knob.** All four drivers already AssertWorktree before work; make the clause mandatory (or default-required with the R-02 surface-to-human path documented). Fix DurationMS (not Duration) and settle the Run(t, newDriver[, opts]) signature.
6. **Scanner-safe scripted prose.** Keep stub-scripted design/review text free of boundary_mock trigger phrasing or pre-declare fixture open_deferrals (sworn#87 false-positives on prose); confirm RunFirstPass accepts spec.json — do not manufacture a spec.md.
7. **D7 confirmed.** AC-03 says "verified"; MergeTrackFn stays nil.
8. **D4 shared stub confirmed** — extend it with per-role scripted outputs (queue/callback) to cover captain + reqverify legs, not just implement/verify.

Flags (not pins): (a) record D1/D4/D7 Type-2 classifications in status.json design_decisions; (b) fused-comment residue in 3 landed files is tracked as sworn#91 — do not touch those files in this slice; (c) use stub-prefixed model IDs for both ImplementerModel and VerifierModel so all legs resolve to the stub; (d) D6 DB-dump pattern citation verified.

§2 decisions D1, D3, D5, D6 clean; D4, D7 acknowledged with citations; D2 amended per pins 1/5. §6 reviewer questions answered (pins 1, 7, 8).

Address pins 1–8 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin 1 is a genuine spec-vs-landed-contract conflict — AC-01 requires Dispatch-level role rejection that ADR-0012 and all four drivers deliberately place at resolution; amending the AC (replan) vs hardening the Type-1 contract is Coach authority, not a determinable fact.
-->
