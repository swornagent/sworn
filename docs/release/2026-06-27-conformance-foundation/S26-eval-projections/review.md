# Captain review — S26-eval-projections
Date: 2026-06-28
Design commit: 092851a15a8b35c53d1b24236d81bc24be83fe67

## Pins

1. [mechanical] §2 (choice #1) / spec in-scope steps 1–2 — design drops the DB-query path the spec's numbered in-scope still lists.
   What I observed: Spec in-scope steps 1–2 say "Opens `.sworn/supervisor-<name>.db`" and "Queries the `events` and `decisions` tables for all dispatches," but design choice #1 reads status.json exclusively and never opens the DB. Verified: the supervisor `events` table schema is `(track_id, release, event, detail, ts)` (supervisor.go:244) — no token/duration/cost columns — and the `decisions` table (S02) is not built yet. The DB physically cannot supply the data; the spec's own "prefer reading from status.json files … authoritative per-slice ground truth" line and AC1 ("non-empty dispatches array") both bless the status.json source.
   What to ask the implementer: Confirm dropping in-scope steps 1–2 is intentional (it is — verified fact). No code path opens the DB for this report; the spec's status.json preference governs.

2. [mechanical] §"Design-level risks" #3 — wrong file/line citation for the Fumadocs fallback.
   What I observed: Risk #3 cites "board.go §49-56" for the `apps/docs/content/docs/release/` fallback. That code actually lives in internal/board/oracle.go (lines 132, 381, 516); internal/board/board.go has no such code. Substance is fine — both this report and ledger.go only handle `docs/release/`, so the Fumadocs prefix is genuinely out of scope for this slice, not a deferral.
   What to ask the implementer: Fix the citation to internal/board/oracle.go, and reframe the "follow-up" note as out-of-scope (matches ledger.go) rather than a bare deferral — or file a tracking issue if it's a real future need (Rule 2).

3. [mechanical] §"Design-level risks" #1 — the temp-dir test fixture pattern is already established.
   What I observed: Risk #1 claims "the test setup pattern for mocking findRepoRoot() and the filesystem walk is not yet established in this package." But cmd/sworn/board_test.go:13–61 already builds a temp-dir repo with per-slice docs/release/<rel>/<slice>/status.json fixtures (t.TempDir + os.MkdirAll + mustWrite); designfit_test.go, run_test.go, and others follow the same shape.
   What to ask the implementer: Reuse board_test.go's fixture pattern. The root-injection plan is sound, but confirm the production entry point resolves the root consistently — telemetryEvents uses os.Getwd() while ledger.go uses findRepoRoot(); pick one.

4. [mechanical] §3 (Files touched) / reuse — ledger.Project already consumes verification.dispatches[].
   What I observed: state.go:109 documents Dispatches as "consumed by ledger.Project (v:2 Records)"; ledger.go already aggregates dispatches per-model (buildCandidateList, pass-rate/cost). The design cites ledger.go only for its glob pattern and never mentions the existing projection.
   What to ask the implementer: Audit whether the per-model aggregation can reuse a shared helper with ledger.Project, or state explicitly why a separate path is justified (this report's metrics — rework rate, mean tokens, mean duration — aren't in the ledger). Avoid a second, silently-drifting per-model aggregation.

5. [mechanical] §2 (choice #2) — adding the `report` case must update the help text in three places.
   What I observed: cmdTelemetry prints "usage: sworn telemetry on|off|status|events" at lines 17 and 31, and the doc comment at line 14 lists the sub-subcommands. Adding `report` without updating all three drifts the help text.
   What to ask the implementer: Update both usage strings and the doc comment when adding the case.

## Summary

Pins: 5 total — 5 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): none — every pin is apply-inline; none ships the slice broken

## Smaller flags (not pins, worth one-line acknowledgement)

(a) Design choice #4 extends the zero-exclusion rule to input_tokens/output_tokens means; spec AC4 mandates it only for duration_ms (in-scope item 6 contemplates absent input_tokens). Reasonable consistency extension — confirm intentional.
(b) cmd/sworn/telemetry.go is also in S02-orchestrator-decision-log's planned_files (T1 track, state planned). Both add a case to the same switch; trivial conflict resolved at merge-track. No action now.
(c) status.json has no design_decisions. No Type-1 choices here (read-only, additive report; no auth/migration/destructive op), no sibling records the field, and no gate reads it — Rule 9 design-fit passes. Noted for the trail.
(d) AC5 fixture: two slices each with one attempt=0 dispatch yields a 0% rework rate; to exercise a non-zero rate include an attempt>0 dispatch. Implementer's test-design call.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Solid, spec-faithful design — the data-source choice is factually well-founded (verified the supervisor DB lacks token/cost columns) and all field references check out. 5 apply-inline pins + 4 flags, all mechanical:

1. **DB-path drop confirmed.** Spec in-scope steps 1–2 (open `.sworn/supervisor-<name>.db`, query events/decisions tables) are correctly dropped — the events table is (track_id, release, event, detail, ts) with no token/duration/cost, and the decisions table (S02) doesn't exist yet. Read status.json exclusively per the spec's stated preference + AC1. Just confirm intent; don't open the DB.
2. **Fix the Fumadocs citation.** Risk #3 cites "board.go §49-56" — the apps/docs/content/docs/release fallback is actually in internal/board/oracle.go (132/381/516). Fix the cite. Since this report and ledger.go both only handle docs/release/, treat the Fumadocs prefix as out-of-scope (not a bare "follow-up" — file an issue if it's a real need, per Rule 2).
3. **Reuse the existing test fixture pattern.** Risk #1's "not yet established" is overstated — board_test.go:13–61 already builds temp-dir per-slice status.json fixtures (t.TempDir + os.MkdirAll + mustWrite); copy that. Root-injection is fine, but pick a single root-resolution convention (telemetryEvents uses os.Getwd(), ledger uses findRepoRoot()).
4. **Audit ledger.Project reuse.** state.go:109 says Dispatches is already consumed by ledger.Project; ledger.go already aggregates dispatches per-model. Either share a helper or state why a separate aggregation is justified (rework rate / mean tokens / mean duration aren't in the ledger). Don't spawn a second drifting path.
5. **Update help text in all three places.** Adding the `report` case means updating both usage strings (lines 17, 31) and the doc comment (line 14) from "on|off|status|events".

Flags (not pins): (a) choice #4 extends zero-exclusion to token means beyond AC4's duration-only mandate — confirm intentional; (b) S02 (T1, planned) also touches telemetry.go's switch — trivial, resolves at merge-track; (c) no design_decisions in status.json, but no Type-1 choices and no gate reads it — design-fit passes; (d) for a non-zero rework rate in the AC5 test, include an attempt>0 dispatch.

§2 decisions 1–6 acknowledged (no memory entries in this project to cite against; all decisions clean). §6 / open items: risks #1–#3 addressed via pins 1–3.

Address pins 1–5 inline during implementation, then proceed to in_progress.

## Triage verdict

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: All 5 pins are apply-inline mechanical fixes (citation, help text, test-pattern reuse, reuse audit, confirm verified DB-path drop); no design re-check needed and no Coach-authority judgement involved.
-->
