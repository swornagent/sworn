# Captain review — S02-claude-subprocess-driver
Date: 2026-07-03
Design commit: 372ceb92c30ddf10cd83b10aaa0c4a81ba9739f1

## Pins

1. [mechanical] §status.json — `design_decisions` field is missing entirely
   What I observed: `status.json` has no `design_decisions` array at all (compare S01-driver-contract's sibling status.json, which records its Type-1 choice with `stake_class`, `options`, `human_decision`, `rationale`). design.md's own §2 lists 9 numbered decisions, and its own closing section flags decision 1 (new `ErrKind` vocabulary) as "the one piece of this slice with the most 'shape' for S03/S04 to inherit — worth a second pair of eyes since it's new contract surface." That is architecturally-significant language written by the design itself, yet no Type-1 record exists to capture a Coach decision on it.
   What to ask the implementer: before code, populate `status.json.design_decisions` for at least decision 1 (new `ErrKind` vocabulary: `config`/`transient`/`provider`/`protocol`, scoped to `internal/driver` because `TestNoWireImports` bars importing `internal/model` — confirmed live at `internal/driver/imports_test.go`). Classify `stake_class` and record the Coach's decision alongside pin 2 below, since they're the same substance.

2. [escalate] — CRITICAL — new `driver.ErrKind` vocabulary drops the Auth/Credits distinction the loop's terminal-halt mechanism depends on, and no planned slice reconciles it
   What I observed: design decision 2 maps a non-zero CLI exit to `ErrKindProvider` uniformly ("classified by what actually happened... not by guessing the cause"), deliberately not distinguishing auth failure from any other provider-side failure. That's a reasonable read of this slice's own AC-04 in isolation. But `internal/run/slice.go:487` has a live, working mechanism — landed 2026-06-28 as `dfb43de feat(run): terminal error halt — KindAuth/KindCredits block before triage` — that calls `model.IsTerminal(implErr)`, type-asserts to `*model.Error`, and short-circuits straight to a BLOCKED verdict specifically for `KindAuth`/`KindCredits` so the engine never wastes a retry/escalation cycle on a condition retrying cannot fix. This mechanism reads `model.ErrorKind`, not the new `driver.ErrKind` string vocabulary S02 introduces.
   Two other planned slices already assume this mechanism survives the transport swap without saying how: S06-loop-dispatch-rewire's spec moves the implement/verify legs onto `Driver.Dispatch` (its own risks section covers verdict-acceptance drift and cross-package test fixtures, but never terminal-error handling). S07-scheduler-failfast's AC-03 literally says: *"If a driver becomes unavailable mid-run (CLI binary removed, auth expired), the dispatch SHALL surface Status=error with its ErrKind through the existing retry/escalation policy"* — citing "auth expired" by name and assuming "the existing... policy" (i.e., the KindAuth fail-fast at slice.go:487) still applies. With S02's vocabulary as designed, an auth-expired claude-cli dispatch becomes indistinguishable from any other provider error once it reaches that check, and the fail-fast-on-terminal-auth behavior silently regresses to wasted retries/escalations the first time someone's claude-cli session expires mid-loop.
   What to ask the implementer: this isn't fixable inside S02's own diff — S02 is correctly implementing its own approved AC-04. The Coach needs to pick one of: (a) amend S02's `ErrKind` vocabulary now to carry an auth/credential-specific value (spec amendment via `/replan-release`, since it changes AC-04's literal mapping), (b) explicitly scope a `driver.ErrKind` → terminal/non-terminal translation into S06 or S07's spec so "the existing retry/escalation policy" S07's AC-03 already promises is actually wired up, or (c) knowingly accept that terminal-halt-on-auth is not preserved through the driver rewire, as a deliberate reliability trade-off recorded in the release's risk register. This needs a decision now, while S02 (and S03, which will copy the same pattern) are still wet cement — not discovered at S06 or S07 time.

3. [mechanical] — design's open item 9 (unattended `claude -p` permission flag) is answered by existing precedent, not still open
   What I observed: design.md item 9 asks whether unattended `claude -p` needs `--dangerously-skip-permissions`/`--permission-mode` for the implementer role's file edits/bash calls to proceed without an interactive approval prompt, and defers the answer to S10's SIT smoke. Baton's shipped bash reference driver (`~/.claude/bin/drivers/claude-cli.sh`) and `captain-handbook.md`'s documented loop dispatches (`claude -p --model 'sonnet[1M]' "/implement-slice S<n> <release>"`) invoke `claude -p` with no such flag, and per project memory ([[project_coach_loop_worktree_hygiene]]) these dispatches perform real file edits in production without one. Since design decision 3 deliberately does not redirect `HOME` (so the Go driver's child process inherits the same credentials/config the bash driver's child does), the precedent should transfer directly.
   What to ask the implementer: confirm this against the bash driver one more time (grep/read only — no dispatch needed) and, if confirmed, downgrade item 9 from "open question for S10" to "resolved by precedent, no flag needed" in the journal — don't let it ride as an unresolved unknown into SIT when it's actually answerable today.

## Summary

Pins: 4 total — 2 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins: #2 — real, evidenced (git history + two sibling specs' own text) reliability regression risk that ships silently if not decided now.

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) Decision 4 (`Roles()` returns `{implementer, verifier}` only, no `captain`) [memory-cited]: aligns with [[project_driver_contract_recut]]'s locked role-universality clause ("any driver may serve any loop role it declares") — declining to declare captain is a scope decision, not a role-universality violation, since neither the spec's user_outcome nor in-scope list names captain-role dispatch. Confirm intentional.
- (b) Decision 8 (registry.go's static `claude-cli` capabilityRegistry entry left at `CapVerify|CapChat`, stale until S05) — verified live at `internal/model/registry.go:17` and the real enforcement point at `internal/run/run.go:353` reads the driver's own `Capabilities()`, not the static table, exactly as design.md claims. S05's own spec already commits to retiring this table entirely. Bounded and accurate as described — no objection.
- (c) Design decision 7 (touching `internal/model/capabilities_test.go` though not a listed touchpoint) — verified live: `cliDriver` appears in both the `TestCapabilities_AllDrivers` table (line 21) and the Chat-capable/no-Chat split lists (lines 50, 84) of `capabilities_test.go`. The same-package, AC-06-required addition is correctly scoped.
- (d) Spec-completeness gate: spec.json's ACs are concrete (literal argv, field names, error-kind values) — not a thin spec. No gate finding here.

## Resolution (post-review, 2026-07-03)

Brad decided pin 2: **Option 1** — extend S02's `ErrKind` vocabulary now
rather than deferring to S06/S07 or accepting the regression. Follow-up
technical fork (how to detect an auth failure on a non-zero CLI exit):
**blunt heuristic** — non-zero exit maps to `ErrKindAuth`, matching
`internal/model/cli.go`'s existing coarse-but-production-proven heuristic
exactly, rejected in favour of stderr pattern-matching because a missed
pattern on CLI wording drift would silently reintroduce the same fail-fast
gap.

Applied to the artefacts:
- `spec.json` AC-04 amended (non-zero exit -> `auth`, not `provider`) + new
  risk R-03 recording the producing-side contract.
- `design.md` decisions 1 and 2 updated; "Risks / open items" section marks
  items 1/2/9 resolved.
- `status.json` now carries the Type-1 `design_decisions` record (pin 1
  addressed in the same pass).
- `S06-loop-dispatch-rewire/spec.json` gets new risk R-03 (the consuming
  side: AC-03's "terminal error kinds -> BLOCKED" must explicitly treat
  `ErrKind==auth` as terminal at both the implement-leg and verify-leg
  consumption points).
- `S07-scheduler-failfast/spec.json` gets new risk R-02 (a pointer noting
  AC-03's "existing retry/escalation policy" depends on S06's R-03 landing
  first — sequencing, not new scope for S07).

Pin 1 and pin 2 are now resolved. Pin 3 (permission-flag question) was
already resolved by precedent at review time. Flag (a) already confirmed.
Routing verdict updated below.

## Suggested acknowledgement reply

TL;DR: solid, well-grounded design — every factual claim checked out live against the repo (imports_test.go, run.go:353, registry.go, capabilities_test.go all match design.md's citations exactly). All pins resolved during design review; nothing left for you to decide, just build against the amended artefacts:

1. **Type-1 decision now recorded.** `status.json.design_decisions` carries the ratified call: `ErrKind` vocabulary is `config`/`transient`/`auth`/`provider`(reserved)/`protocol`; non-zero CLI exit maps to `auth`, matching `internal/model/cli.go`'s existing heuristic exactly (Brad, 2026-07-03).
2. **Build AC-04 as amended, not as originally drafted.** `spec.json` AC-04 now reads non-zero exit -> `ErrKindAuth` (not a generic `provider` label) — see design.md decision 2 for the full rationale (preserves `internal/run/slice.go:487`'s terminal-halt-on-auth fail-fast once S06 wires this in). `design.md`'s "Files touched" / AC traceability table is otherwise unchanged.
3. **Item 9 closed, no flag needed.** Confirmed by precedent (baton's bash `claude-cli.sh` / captain-handbook loop dispatches run `claude -p` unattended with no skip-permissions flag, in production). Nothing to do here.

Flags (informational, no action needed): (a) `Roles()` scope confirmed against [[project_driver_contract_recut]]; (b) registry.go stale-entry bridge to S05 confirmed bounded; (c) capabilities_test.go touch confirmed correctly scoped; (d) spec is concrete, not thin; (e) S06 and S07 specs now each carry a matching risk entry (S06 R-03, S07 R-02) so the consuming side of the `auth` contract isn't lost at those slices' own design review.

§2 decisions 2 (as amended), 3, 5, 6 acknowledged clean. §6/Risks-for-Captain items 1, 2, 8, 9 all resolved — see design.md's updated "Risks / open items" section.

Proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: pin 2's Coach decision is made and applied to spec.json/design.md/status.json (this slice) and S06/S07 (forward risk notes); pin 1 (design_decisions) and pin 3 (permission flag) resolved in the same pass. Nothing left requiring Coach authority.
-->

