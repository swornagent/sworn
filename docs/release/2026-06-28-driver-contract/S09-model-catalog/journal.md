# Journal — S09-model-catalog

## 2026-07-10 — Design TL;DR session

Track `T5-catalog` had no worktree yet; materialised
`track/2026-06-28-driver-contract/T5-catalog` from `release-wt/2026-06-28-driver-contract`
tip (cec87c7, T4-resolution-loop already merged, dependency gate clear —
`dependsOnTracks: [T4-resolution-loop]`, `blocked: false` per board oracle).

Read `spec.json`, the release `intake.md` N-11/A-05 decision history, the
landed `internal/driver/registry` (S05) and `internal/model.ProviderConfig`
(S08 pricing lives in `internal/model/client.go` `PriceForModel`), and the
existing `cmd/sworn/capabilities.go` verb this new one sits alongside.

**Key design finding**: AC-01's "determined via registry enumeration" can't
be read literally — `internal/driver/registry.Default()` only registers 4
driver entries and explicitly excludes Google/Ollama by design (its own
header comment: "verify-only providers... stay on the one-shot utility
path"), but S09's own touchpoints require exactly those two providers in the
catalog. Extending the registry is out of scope for this slice. Resolved as
design decision D1: `catalog.go` runs its own no-dispatch credential check
against `model.ProviderConfig` uniformly across all 7 providers, rather than
partially reusing the registry for 5 and side-channeling the other 2. Full
rationale in `design.md`.

Three more Type-2 decisions recorded (D2: Ollama's per-model `/api/show`
call, N+1 against a local daemon; D3: Ollama always attempted, keyless,
mirrors `claude-cli`'s registry precedent; D4: Google's `tools` annotation is
unconditionally `unknown` — `supportedGenerationMethods` has no explicit
tool-support signal per spec.json's own rationale). All four are narrow/local
to this one new package+command, self-classified Type-2, no Type-1 human
ratification required per Rule 9 — flagged to the Captain in `design.md` and
`status.json.design_decisions` for review anyway.

**Rule 2 deferral**: pricing display (OpenRouter's `pricing` block is
free/wire-honest and could annotate `$/1M`) is left out entirely — no AC
requires it, and `PriceForModel` is keyed by the registry's fully-resolved
`provider/model` ID, not catalog's raw per-provider wire IDs, so wiring it in
would need its own normalisation pass. Why: no AC requires it, adding an
unasked capability-shaped surface risks scope creep. Tracking: none filed —
raising to the Coach at design review whether a follow-on issue is wanted.
Acknowledgement: pending this design review.

No production code written this session (Rule 9 gate — design review
precedes implementation). `design.md` written; `status.json` ->
`state: design_review`.

Next: `/design-review S09-model-catalog 2026-06-28-driver-contract`, then
Coach acknowledgement (`DECISION: PROCEED`) before implementation resumes.
