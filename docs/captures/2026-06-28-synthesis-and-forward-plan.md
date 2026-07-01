# Session synthesis + forward plan — coach loop as reference, Go sworn as replication target

**Date:** 2026-06-28. The consolidating entry point for the day's work. Indexes the detailed captures,
states the durable assertions in one place, and sets the forward stance for the full sworn build.

## 0. The organizing stance (Brad, 2026-06-28)

**The bash coach loop is the reference working model; the Go sworn engine is the replication target,
validated against it.** Rationale: coach loop is the farthest-advanced, demonstrably-working
implementation (it cold-starts, drives a real release to genuinely-correct `verified` slices, banks
work, isolates failures). Refining coach loop is still worthwhile precisely because it is the reference:
every behaviour we harden there becomes the executable spec the Go port must reproduce and be validated
against. So two tracks run in parallel — **refine the reference (coach loop)** and **replicate + validate
in Go (sworn)** — with the Go correctness criterion being "does it reproduce the reference's behaviour on
the same inputs."

## 1. Capture index (read these for detail)

- `2026-06-28-sworn-eval-findings.md` — Go three-model dogfood; 12+ cold-start/wiring/driver bugs; model verdicts.
- `2026-06-28-bash-vs-go-eval.md` — same release/slices, Go vs bash; same-model contrast (harness, not model).
- `2026-06-28-bash-coachloop-learnings.md` — per-bug "how bash avoids it" + Go ports; merge-stall + fix; interpreter/escalation design.
- `2026-06-28-sworn-architecture-recommendation.md` — resilience/perf/memory/elegance; the verification-gap (§3.5), greenfield harness (§3.6), pre-merge UAT (§3.7).
- `2026-06-28-autonomous-loop-benchmark-landscape.md` — benchmark landscape; loop-delivery score; SWE-bench + SWE-AGI PoCs.
- `S27-parallel-dispatch-fix/` — the two interim Go fixes folded into the release as a tracked slice.
- (private) `sworn-internal/docs/strategy/2026-06-28-loop-delivery-benchmark-commercial.md` — category/moat/wedge.

## 2. Consolidated assertions (what we now hold to be true)

### A. State of the two engines
1. **coach loop (bash) works end-to-end** and is the reference: cold-starts a planned release, dispatches
   real agents via a driver contract, banks work per dispatch, isolates track failures, and drives slices
   to genuinely-correct `verified` (independently re-tested: S01/S08/S13 + more, wired + tested, no gaming).
2. **sworn (Go) parallel loop is DOA on the cold path** — ~12 cold-start/wiring/driver bugs. Its
   *deterministic core* (router, scheduler, oracle, state machine, worktree materialisation) is sound; the
   *agentic dispatch/verify/resilience layer* is not.

### B. Root-cause architecture
3. **The harness is the decisive variable, not the model.** Same `deepseek-v4-pro`: Go loop → 0 verified;
   bash loop → correct, integrated, verified slices.
4. **Go's root failure: it reimplemented the agent loop + per-provider wire format in-process** (and
   assumed Driver-1 bootstrap it never performed). One struct tag (`content,omitempty`) broke every
   provider. Fix = a **Driver contract**: delegate the agent loop to a driver; the orchestrator never sees
   a wire message.
5. **coach loop's elegance is a thin deterministic orchestrator over a runtime-driver contract.** That IS
   the reference architecture for the Go re-build ("one orchestrator, N drivers").

### C. The quality / verification spine
6. **unit-green ≠ delivery-green.** A per-slice verifier (even a full agentic, test-re-running one) sits at
   the unit/component tier; it structurally cannot see a SIT- or UAT-class defect. Proof: coach loop built
   sworn DOA *with* the full verifier.
7. **Test-level map:** per-slice verifier ≈ unit; Baton **Rule 1 ≈ SIT** (reach the real integration
   point); Baton **Rule 10 ≈ UAT** (no-mock, real output). Baton encodes these; they were not wired as
   loop gates, so sworn shipped on unit-green.
8. **Greenfield is where the loop is most dangerous** (no harness to tap into; Fired has Playwright, sworn
   did not). Source fix: the **Planner establishes the E2E/SIT harness as Slice 0**; feature slices
   depend on it; Rule-1/Rule-8 fail closed until it exists. CLI/library SIT floor = "boot the assembled
   binary and run the affordance."
9. **Pre-merge guided human walkthrough (UAT)** before `release → base`: engine generates the route from
   touched Rule-10 journeys + impact analysis; optional + overridable but the override is a logged Rule-2
   decision. Complements `sworn ship` (post-merge real-infra attestation).
10. **"Verify in the loop" only works on externally-owned, agent-immutable ACs.** Proof: forced to pass a
    held-out test, a loop agent fabricated its own copy (flask-4992); SWE-AGI's frozen tests/scaffold are
    the correct shape.
11. **The loop is only as good as the ACs it can verify against** — quantified: 100% of *visible* public
    tests yet 76–83% of the *full* private suite. Closing the gap needs spec-grounded verification beyond
    the given tests.

### D. The orchestration / interpreter layer
12. **The interpreter must be more than a classifier.** Today it returns DONE/RETRY/BLOCKED; faced with an
    agent's confirmation/clarification it can only RETRY → infinite loop (live merge-track stall).
13. **Three-tier confirmation/escalation with session continuity:** interpreter (haiku) classifies →
    **Captain** adjudicates the ambiguous/confirmation cases in-session (answer when a deterministic gate
    authorizes, else escalate) → human **PAGE only when needed, with the worker session HELD OPEN**
    (checkpoint-and-resume; `claude --resume` / transcript replay) so a page never costs a full rework.
14. **Merge (and similar) decisions should be deterministic gates** (the board oracle's verified-check),
    not an agent asking permission. The merge-stall root cause was a dangling `BATON_AUTO_CONFIRM` flag the
    loop passed but no command honoured — fixed in `merge-track.md` (interim); the three-tier design is the
    long-term form.

### E. Resilience / engine behaviours to replicate (from coach loop → Go)
15. **Serialized cold-start before fan-out** (bash `ensure_release_worktree` once, committed, before any
    worker). 16. **Auto-WIP-commit before every dispatch** (never reset on retry). 17. **Track-local
    failure → PAGE, never phase-wide cascade.** 18. **Per-role model config + escalation from config**
    (never hardcoded `openai/*`). 19. **Driver owns the loop + stop condition** (exit on "no tool calls",
    turn cap as a backstop; force a summary on empty text). 20. **Tool-exec cache/env hygiene** (GOCACHE/
    GOMODCACHE/HOME outside the worktree).

### F. Benchmarks + models
21. **No public benchmark measures the loop** — all measure the model-in-a-harness. Opportunity: a
    **loop-delivery score** (time-horizon × completion × delivery-correctness-through-gates × efficiency).
22. **SWE-bench under-measures sworn** (hides ACs → measures inference); **SWE-AGI fits** (explicit,
    immutable ACs → measures adherence). Adapters for both exist and are reusable.
23. **Model verdict (this build):** `deepseek-v4-pro` is the verified-per-dollar standout (76% SWE-AGI for
    ~1/800th the cost; swept the Go release slices via bash). `claude-sonnet` higher coverage (83%) at far
    higher cost. `gpt-5.3-codex` unusable in this build (HTTP 520 + Responses-API bug).

## 3. Forward plan

### Track A — refine the reference (coach loop) [farthest advanced; keep it the spec]
- Land the interpreter upgrade: classifier → bounded in-session responder; three-tier Captain escalation;
  session held-open-on-page (checkpoint/resume). (Merge-stall interim fix already in.)
- Make merge/accept a deterministic oracle gate end-to-end (retire the agent-asks pattern).
- Wire the acceptance spine: Planner Slice-0 harness establishment; Rule-1 SIT gate; pre-merge UAT walk.
- Every behaviour hardened here is a line item in the Go validation suite (below).

### Track B — replicate + validate in Go (sworn)
- Re-architect on the **Driver contract** (delegate the agent loop + wire format; default to a subprocess
  agent driver; one hardened in-process oai driver as an option). This subsumes most model-layer bugs.
- Replicate the engine behaviours #15–#20 (serialized bootstrap, auto-WIP, track-local failure, config
  escalation, driver-owned stop, cache hygiene).
- **Validate against the reference, differentially:** same release + same inputs through coach loop and
  through sworn; assert the Go engine reproduces the reference's routing decisions, state transitions,
  bank/commit behaviour, and final verified set. Divergence from the reference is a Go bug.
- Apply the acceptance spine to sworn's OWN build (dogfood with the SIT floor + AC-explicit verification),
  so sworn is never again shipped on unit-green.

### Cross-cutting — proof + economics
- Stand up the **loop-delivery score** (SWE-AGI suite + a sworn-native AC-explicit suite); report
  verified-per-dollar per model from FT-7 telemetry; use the full-loop − implement-only delta as the
  loop-lift proof point.

## 4. Open items / not-yet-done (so nothing is lost)
- Full-loop (independent verify + retry/escalate) SWE-AGI run for the loop-lift delta — designed, not run.
- Restart the deepseek sworn build post-merge-fix to confirm the fix end-to-end + finish the release.
- merge-release left human-triggered by design (no change); confirm that stays the intended posture.
- The three-tier interpreter/Captain design is captured but not yet implemented in coach loop or Go.
- Driver-contract re-architecture for Go is recommended, not yet sliced.

## 5. Live-build finding (2026-06-28) — recurring touchpoint conflicts (planning + invariant-2 gap)

The deepseek build hit the SAME class of BLOCK three times: `internal/model/oai.go` (and other
`internal/model/*`) is modified by T2-model-layer, T3-agentic-verifier, AND T7-telemetry-eval, but the
touchpoint matrix declared none of them shared. Each track branched before T2 merged, so each collides at
merge → BLOCKED → /replan. Diagnosis:
- **Planning-fidelity gap (Rule 8/9):** the planner under-declared the shared surface. `internal/model/oai.go`
  is genuinely shared across the three model-touching tracks (T2 owns the drivers; T3's verifier calls models;
  T7's dispatch reads tokens/cost) — it should have been DOCUMENTED SHARED, or T3/T7 sequenced depends_on T2.
- **Invariant-4 works; invariant-2 is missing:** the merge-time conflict detector correctly catches and BLOCKs;
  there is no dispatch-time touchpoint-disjointness enforcement to PREVENT overlapping tracks running in
  parallel (the conformance release's own S06 is exactly this gap).
- **The restored design-review gate is the early catch:** the Captain reviews design.md's planned touchpoints
  and can flag cross-track overlap BEFORE code — turning a merge-time BLOCK into a plan-time amendment. An
  argument for the §3.8 detailed-design-role scrutiny extending to touchpoint completeness, not just AC
  completeness.
- **Interim handling:** each conflict resolved by the agreed not-human-authored auto-replan (forward-merge
  release-wt into the track, combine both sides on oai.go, document the shared file, clear the block, resume) —
  T7/S25 and T3/S12. The durable fix is a proper /replan-release declaring internal/model/* shared across
  T2/T3/T7 (or sequencing them) so the recurrence stops.
