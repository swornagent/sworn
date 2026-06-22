# Captain review — S51-cli-command-registry
Date: 2026-06-22
Design commit: b4177cee639f66e40b9e8a0fa5390f7c0df468d9

## Pins

1. [mechanical] §3 / planned_files — `cmd/sworn/verify.go` is an undeclared touchpoint.
   What I observed: §2.1 + §3 relocate `cmdVerify` and `openDeferralsFlag` (today at
   `cmd/sworn/main.go:146` and `:138` on release-wt) into a new `cmd/sworn/verify.go`.
   That file is NOT in status.json `planned_files` (5 entries) nor in the index.md T15
   touchpoint rows. The Verifier's Gate 2 (diff vs planned_files) and the S30 lint-touchpoints
   gate will both flag an undeclared file.
   What to ask the implementer: add `cmd/sworn/verify.go` to status.json `planned_files`
   and to the T15 rows of the index.md touchpoint matrix (it is T15-owned, collides with
   nothing — verify.go is absent on release-wt and no sibling claims it), OR record it under
   proof.md "Divergence from plan". Declaring it is cleaner.

2. [mechanical] §3 / spec Risk 3 — commands_test.go must assert non-empty Summary.
   What I observed: spec Risk 3's mitigation is "commands_test.go asserts every registered
   command has a non-empty Summary" (so `usage()` derived from `command.All()` never emits a
   blank help line). The design's §3 integration test only "asserts resolution and handler
   identity" — the Summary assertion is not in the plan.
   What to ask the implementer: add an assertion to `commands_test.go` that every
   `command.All()` entry has a non-empty `Summary`, satisfying the spec Risk 3 mitigation.

3. [mechanical] §2 / Step 2b designfit (Rule 9) — status.json `design_decisions` is absent.
   What I observed: `design_decisions` is `<ABSENT>` in status.json. S51 introduces an
   architecturally-significant pattern (a process-wide command registry). The core choice
   (registration pattern over the switch) was a Coach decision this session; the §2 items are
   mechanical sub-decisions. The S32-designfit-decisions-gate fails closed when Type-1 work is
   implied but `design_decisions` is empty.
   What to ask the implementer: populate `design_decisions` to record the Coach-decided
   registry-pattern choice (Type-1, decided) so `sworn designfit 2026-06-19-safe-parallelism`
   passes; or confirm designfit treats this slice as benign-empty.

## Summary
Pins: 3 total — 3 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: none (none ship the slice broken; all are apply-inline gate-hygiene fixes)

## Smaller flags (not pins, worth one-line ack)
- (a) §2.5: `usage()` keeps hand-written per-verb prose paragraphs; only the command *listing*
  is generated from `command.All()`. This meets AC "help lists every registered command" but
  is not a fully registry-driven usage. Confirm the partial generation is intentional.
- (b) Downstream handoff (informational, already gated): when T15 merges to release-wt, T3's
  S07-paging implement re-entry must resolve `cmd/sworn/main.go` by converting its login/account
  cases to `command.Register(...)`. This is gated by the `T3 depends_on T15` edge added this
  session, so T3 holds until T15 merges — no action for this slice.
- Memory: design adds no new dependencies (pure stdlib registry) — aligns with
  [[project_dep_policy]] ("minimal justified deps"); no dep justification needed.

## Suggested ack reply
<!-- Coach-extractable section -->

TL;DR clean design, faithful to the approved registration plan — central registration keeps S51 touchpoint-disjoint from every in-flight track. 3 mechanical pins + 2 flags, all apply-inline:

1. **Declare verify.go.** §2.1/§3 move cmdVerify + openDeferralsFlag into a new cmd/sworn/verify.go. Add it to status.json planned_files AND the T15 rows of the index.md touchpoint matrix (it's T15-owned, collides with nothing), or document it under proof.md "Divergence from plan". Otherwise Gate 2 + the S30 lint-touchpoints gate flag an undeclared file.
2. **Assert non-empty Summary.** Spec Risk 3 mitigation: commands_test.go must assert every command.All() entry has a non-empty Summary (no blank help lines), not just resolution/handler identity. Add that assertion.
3. **Populate design_decisions.** status.json design_decisions is empty; S51 introduces an architecturally-significant registry pattern (Coach-decided this session). Record that decision so sworn designfit passes — or confirm it's benign-empty.

Flags (not pins): (a) §2.5 usage() keeps hand-written per-verb prose, only the listing is registry-generated — confirm intentional; (b) downstream, T3/S07 resolves main.go→register on its re-entry, already gated by the T3 depends_on T15 edge — no action here.

§2 decisions 1–5 ack (all mechanical relocation/struct-shape choices; no memory conflict; no new deps). §6 (none) ack.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 3 pins are apply-inline gate-hygiene fixes (declare a relocated file, add a test assertion, populate design_decisions); none changes the design or requires Coach judgement.
-->
