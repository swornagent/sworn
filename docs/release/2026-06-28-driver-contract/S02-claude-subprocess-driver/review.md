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

## Suggested acknowledgement reply

TL;DR: solid, well-grounded design — every factual claim checked out live against the repo (imports_test.go, run.go:353, registry.go, capabilities_test.go all match design.md's citations exactly). 3 pins + 4 flags, one of which (pin 2) is a real cross-slice reliability gap worth resolving before S03 copies the same vocabulary shape:

1. **Record the Type-1 decision.** `status.json` has no `design_decisions` entry at all — add one for the new `ErrKind` vocabulary (decision 1), and fold the Coach's call on pin 2 into the same record.
2. **Terminal-halt-on-auth gap (CRITICAL, needs a Coach call, not an inline fix).** The new `driver.ErrKind` vocabulary collapses auth failures into generic `provider`, but `internal/run/slice.go:487`'s live `model.IsTerminal`/KindAuth fail-fast — which S07-scheduler-failfast's own AC-03 explicitly assumes still works ("auth expired... through the existing retry/escalation policy") — reads a different vocabulary entirely. Pick one: amend S02's AC-04 to carry an auth-specific `ErrKind` now, scope a translation layer into S06/S07, or knowingly accept the regression. See review.md pin 2 for the full trade-off — this needs deciding before S02/S03 ship the pattern, not discovered at S06/S07 time.
3. **Close out item 9, don't defer it.** The permission-flag question already has a precedent answer (baton's bash `claude-cli.sh` / captain-handbook loop dispatches run `claude -p` with no skip-permissions flag and it works in production, per memory). Confirm by reading the bash driver, then mark it resolved rather than carrying it as an open unknown into S10.

Flags (not pins): (a) `Roles()` scope decision confirmed against [[project_driver_contract_recut]] — no objection; (b) registry.go stale-entry bridge to S05 confirmed bounded and accurate; (c) capabilities_test.go touch confirmed correctly scoped; (d) spec is concrete, not thin.

§2 decisions 2, 3, 5, 6 (error-mapping literalism, env hygiene, argv-per-AC discipline, defensive envelope parsing) acknowledged clean. §6 has no explicit questions section beyond the two flagged in design.md's own "Risks / open items" — both addressed above (item 8 → flag (b), item 9 → pin 3).

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: pin 2 is a genuine reliability trade-off (preserve auth/credits fail-fast through the driver rewire vs. accept the regression vs. amend AC-04 now) that spans S02/S03/S06/S07 and requires a Coach pick, not an inline implementer fix.
-->

