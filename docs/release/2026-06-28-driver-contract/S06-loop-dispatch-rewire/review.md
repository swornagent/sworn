# Captain review — S06-loop-dispatch-rewire
Date: 2026-07-10
Design commit: 2c87739de812f3bfec89bc5b37ff4ccf38f5c1fc

## Pins

1. [escalate] §Key-decisions.D2 — Captain-leg resolution failure routes to fail-open deferral, narrowing AC-02's "any role leg" hard-error contract.
   What I observed: AC-02's text: "If Resolve fails for ANY role leg, RunSlice SHALL return a descriptive error ... BEFORE any model dispatch." D2 routes captain-leg resolution failure through the existing `recordDesignGateDeferral` fail-open path (slice.go:141, tracked sworn#51) instead — the design's own words: "This reading is flagged for the Captain because AC-02 says 'any role leg'." Verified against live code: no driver declares RoleCaptain today (claude.go:33 declares implementer+verifier only; codex_test.go:369 asserts !RoleCaptain), so under a hard-error reading every claude-cli/codex-first run halts at the design gate; under the deferral reading the run proceeds with a durable Rule 2 record, preserving today's semantics class (slice.go:336-347 already defers on captain agent-construction failure). Both readings are internally consistent; the choice changes the slice's contract with AC-02 as written.
   What to ask the implementer: nothing — this is a Coach decision. Option (i) accept the deferral reading: captain resolution failure = Rule 2 deferral + proceed (AC-02's hard-error contract applies to implement/verify legs), recorded as a design_decision citing the Coach acknowledgement, optionally with a /replan-release AC-02 text amendment. Option (ii) hard-fail all three legs per AC-02's literal text, accepting that subprocess-first runs halt at the design gate until subprocess drivers declare captain. Option (i) prioritises run survivability on subprocess drivers; option (ii) prioritises literal AC fidelity and gate visibility. The Coach picks.

2. [mechanical] §Approach (fail-fast resolution) — The registry's role-failure error does not name the model ID; AC-02 requires "naming the model, role, and registered alternatives."
   What I observed: design says the registry error "already names model, role, and registered alternatives — S05 AC-02/AC-03 vocabulary." Verified live (registry.go:160-164): the role-arm error is `"registry: driver %q cannot serve role %q — declared roles: %s; drivers declaring %q: %s"` — driver name, role, alternatives, but NOT the model ID; the prefix-arm (registry.go:156-158) names the prefix, not the full model ID, and no role. The registry error alone does not satisfy AC-02's error contract.
   What to ask the implementer: wrap every upfront Resolve failure at the RunSlice call site — e.g. `fmt.Errorf("RunSlice: resolve %q for role %q: %w", modelID, role, err)` — and make TestRunSliceResolutionFailure assert all three of model ID, role, and registered alternatives appear in the returned error text.

3. [memory-cited] §Key-decisions.D3 — Terminal set {auth, credits} with one contract predicate honours the S04 tracked obligation on this slice.
   What I observed: S04's recorded design_decision D5 carries "Rule-2 tracked obligation on S06-loop-dispatch-rewire: the rewired loop MUST treat Result.ErrKind in {auth, credits} as terminal" (S04 status.json, Coach-acked via T3 captain-proceed.md 2026-07-10; also spec R-03's binding text). D3 delivers exactly this: `driver.TerminalErrKind` = {ErrKindAuth, ErrKindCredits}, the private `errKindCredits` (inprocess.go:40) promoted to the contract package, consumed at both the implement leg (replacing model.IsTerminal at slice.go:492) and the verify leg's BLOCKED mapping, with the four halt tests plus the transient-continues case.
   What to ask the implementer: acknowledge the citation; carry it into the TerminalErrKind doc comment and the design_decisions record (pin 6) so the obligation's closure is traceable.
   Citation: [[project-driver-contract-recut]] (S04 status.json design_decisions D5 is the primary record).

4. [memory-cited] §Key-decisions.D6 — Single proxy predicate behind FromEnv, ResolveLoopClient, and the registry protects the memory-validated aggregator/proxy journey.
   What I observed: verified live that the drift D6 closes is real: `InProcess.newClient` defaults to proxy-blind `model.NewClient` (inprocess.go:80,86) while `registry.proxyRouting()` (registry.go:381-392) independently re-implements FromEnv's login condition (config.go:66-94) to advertise ViaProxy — post-rewire, capabilities would claim proxy while dispatch goes direct, the exact R-04 regression. D6's `model.ProxyRoute` extraction + `ResolveLoopClient` + registry delegation makes both surfaces evaluate literally the same function, and the three-part reachability test (advertise / server-side-observed dispatch / SWORN_DIRECT flips both) is the binding artefact R-04 demands. This serves the journey Brad validated firsthand (workers on OpenRouter) and moves toward the ratified wire-vs-usage service-layer direction.
   What to ask the implementer: acknowledge the citations; proceed as designed. Note the predicate routes bearer credentials to endpoints — keep the SWORN_PROXY_URL override test-only per the S04-era Coach ack (config.go:66 "credential-trust boundary").
   Citation: [[project_aggregator_proxy_validation]], [[project_model_layer_service_refactor]].

5. [mechanical] §Key-decisions.D7 — ProviderConfigFromEnv SWORN_* widening is behaviour outside the touchpoints; prove it stays additive with a canonical-wins test.
   What I observed: verified the factual premise: provider.go's ProviderConfigFromEnv aliases only OPENAI (SWORN_OPENAI_API_KEY) and GOOGLE today, while FromEnv's direct arm builds its config from the SWORN_* namespace — so an unwidened default registry would drop direct dispatch for SWORN_*-only environments (the documented worker setup). The widening (`envOrAlias(CANONICAL, SWORN_CANONICAL)` for every provider key) is required to avoid regressing today's loop reach, and is strictly additive since canonical wins. It also changes `sworn capabilities` output for SWORN_*-only environments (truthfully). The design correctly flags it as beyond the literal touchpoints; the track-merges-as-one-unit containment applies.
   What to ask the implementer: add a precedence test covering every widened key — both CANONICAL and SWORN_CANONICAL set, assert the canonical value is used — so "additive, canonical wins" is proven, not asserted; declare the file in the proof bundle's files-changed as the design already does in "Files to touch."

6. [mechanical] §Key-decisions (all) — status.json has no design_decisions record; the design-fit gate fails closed on it before in_progress.
   What I observed: S06's status.json contains no `design_decisions` field at all, while verified siblings S04/S05 both carry populated records with stake classification and human_decision citations (Rule 9). The design carries at least five decisions needing recording: D1 (registry injection shape — planning-intake decision), D2 (pin 1's Coach decision), D3 (S04-obligation closure), D6+D7 (R-04 binding + this review's acknowledgement).
   What to ask the implementer: populate `design_decisions` in status.json before the design_review → in_progress transition, citing the existing human decisions (planning intake 2026-07-02 for D1; S04 captain-proceed.md 2026-07-10 for D3; this review's Coach acknowledgement for D2/D7) per the S04/S05 record shape.

Pins: 6 total — 3 [mechanical], 2 [memory-cited], 1 [escalate]
Critical pins (if any): 1 (the AC-02 reading decides gate semantics for every subprocess-first run; an undecided deviation ships either bricked claude-cli runs or a verifier-visible spec breach)

## Summary

Pins: 6 total — 3 [mechanical], 2 [memory-cited], 1 [escalate]. Critical: pin 1. Every cited symbol and line anchor in the design verified accurate against live worktree code except one (pin 2's error-text paraphrase); all four spec risks are answered with binding, testable mechanisms; cross-release ancestry on the touchpoints is clean.

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) D9's InterpretVerifier-deletion worry (design risk 3) is resolved determinable-safe: `internal/run` is an internal package — nothing outside this module can set the field — and repo grep confirms zero readers beyond the declaration; delete without a compat shim.
- (b) Pre-existing fused comment at slice.go:694 (`// ── Dispatch agentic verifier ...── // Create an agent...`) — the known newline-eating pattern, harmless here (comment+comment) but in the exact region this slice rewrites; repair it during the verify-leg rewrite and run the corruption grep + `gofmt -l` sweep from the test plan after editing.
- (c) D8's _test.go-inclusive scan means internal/verify's wire-type-heavy test stubs (verify_agentic_test.go's Chat/ChatStructured fakes) must become fake-driver-based — keep the acceptStructuredVerdict-level assertions minimally diffed so R-01's "any behaviour change must fail an existing test first" property survives the transport swap.
- (d) Design risk 4 (leg-function signature breaks) is acceptable as stated: all callers are module-internal and enumerated in "Files to touch"; no exported-API compatibility promise exists.
- (e) S07/S08 (same track, planned) share the slice.go/scheduler surfaces; T4's serial track order resolves sequencing — no depends_on edits needed.

## Suggested acknowledgement reply

TL;DR High-fidelity design — every cited symbol verified live, all four spec risks answered with binding mechanisms; one error-text inaccuracy and one spec-deviation decision. 6 pins + 5 flags:

1. **D2 captain-leg AC-02 reading.** The fail-open deferral for captain-leg resolution failure stands as designed: AC-02's hard-error contract applies to the implement and verify legs; a captain resolution failure records the registry's descriptive role error inside a Rule 2 deferral via recordDesignGateDeferral and proceeds. Record this as a design_decision on status.json citing this acknowledgement as the human decision, and note the AC-02 narrowing there explicitly so the verifier reads it as decided, not drifted.
2. **Resolve errors must name the model.** The registry's role-arm error names driver/role/alternatives but NOT the model ID (registry.go:160-164). Wrap every upfront Resolve failure at the RunSlice call site — `fmt.Errorf("RunSlice: resolve %q for role %q: %w", modelID, role, err)` — and make TestRunSliceResolutionFailure assert model ID, role, and registered alternatives all appear.
3. **Terminal-set citation.** D3 closes the S04 D5 tracked obligation ({auth, credits}, T3 captain-proceed.md 2026-07-10). Cite that record in the TerminalErrKind doc comment and in the design_decisions entry so the obligation's closure is traceable.
4. **Proxy predicate unification confirmed.** D6 proceeds as designed — [[project_aggregator_proxy_validation]] and [[project_model_layer_service_refactor]] both cited and honoured; the three-part reachability test (advertise / server-side-observed dispatch / SWORN_DIRECT flips both surfaces) is the binding R-04 artefact. Keep SWORN_PROXY_URL test-only per the credential-trust boundary ack.
5. **D7 canonical-wins test.** The SWORN_* widening proceeds (required to not regress SWORN_*-only worker setups). Add a precedence test for every widened key — both CANONICAL and SWORN_CANONICAL set, canonical value wins — so "strictly additive" is proven, not asserted.
6. **Record design_decisions.** Populate status.json design_decisions before the in_progress transition: D1 (planning intake 2026-07-02), D2 (this acknowledgement, pin 1), D3 (S04 captain-proceed.md 2026-07-10), D6+D7 (R-04 binding + this acknowledgement), per the S04/S05 record shape — the design-fit gate fails closed on an empty record.

Flags (not pins): (a) InterpretVerifier deletion is safe — internal-package visibility plus zero repo readers; delete without a shim; (b) repair the pre-existing fused comment at slice.go:694 while rewriting the verify leg, then run the newline-corruption grep + gofmt -l sweep; (c) D8's _test.go-inclusive scan turns verify_agentic_test.go's wire-type stubs into fake drivers — keep acceptStructuredVerdict-level assertions minimally diffed to preserve R-01's regression property; (d) signature breaks are module-internal with all callers enumerated — acceptable; (e) S07/S08 sequencing resolves by T4's serial track order.

§2 decisions D1, D4, D5, D8, D9, D10 acknowledged clean; D3 and D6 acknowledged with citations (pins 3, 4); D2 and D7 decided per pins 1 and 5. §6 reviewer-risk items 1–4 addressed by pins 1 and 5 and flags (a) and (d).

Address pins 1–6 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: yes
REASON: Pin 1 — D2 narrows AC-02's "any role leg" hard-error contract to a captain-leg fail-open deferral, a spec deviation only the Coach can accept or redirect; all other pins are apply-inline. Constitutional flag: D6/D7 change credential-based routing (which bearer token/endpoint a dispatch uses).
-->
