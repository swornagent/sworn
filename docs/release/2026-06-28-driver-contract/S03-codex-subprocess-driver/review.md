# Captain review — S03-codex-subprocess-driver
Date: 2026-07-06
Design commit: a925a05a2eac6ea67e9984c0da0a8fa512ebfabc

## Pins

1. [escalate] — CRITICAL — §Key design choices, decision 1 — codex verifier dispatch has no fresh-context equivalent to claude's `--no-session-persistence`, and this is a spec omission, not a deliberate scope decision
   What I observed: decision 1 states "No `--no-session-persistence`-equivalent flag is added for the verifier role — neither AC-01 nor AC-02 names one for codex, unlike claude's AC-03, so I'm not inventing one." This is true as far as it goes, but S02's own spec.json AC-03 (verified, shipped, same track) reads: "the driver SHALL pass `--no-session-persistence` (fresh context, Rule 7)" — i.e. that flag exists specifically to satisfy Rule 7 (Adversarial Verification), which requires the verifier to run with no inherited state from the implementer's session. S03's spec.json AC-02 covers the same verifier-dispatch shape (schema-in-prompt, `StructuredJSON` population, fail-closed on non-JSON) but is silent on fresh-context. Because S03's own spec's R-01 already documents that codex's CLI flags/envelope are a genuine unknown (no live binary exercised), whether `codex exec` has any default session/rollout persistence that could leak state across invocations in the same worktree is not determinable from this repo — it needs either Coach knowledge of codex's actual CLI behaviour or a documented assumption. The design resolves the gap by inference from spec silence rather than by treating it as a Rule-7 compliance question, and it isn't surfaced anywhere in design.md's own "Risks / open items" section — this is exactly the "§6 is a floor, not a ceiling" case.
   What to ask the implementer: do not resolve this by inference. Either (a) confirm from codex CLI documentation/help output that a bare `codex exec` invocation is stateless by default (no session file written or read unless a `resume` subcommand is used) and record that confirmation in journal.md with a citation, or (b) treat this as a spec gap and get spec.json AC-02 amended via `/replan-release` to require an explicit fresh-context flag/assumption mirroring S02 AC-03's Rule-7 framing. This is a Coach decision because it turns on live CLI behaviour this review cannot verify from the repo, and it affects the integrity of every future codex-driven verifier dispatch, not just this slice's tests.

2. [memory-cited] — §Key design choices, decision 6 / `status.json` `design_decisions[0]` — the open question is already answered by a binding cross-driver contract; the implementer's proposed default is correct
   What I observed: decision 6 flags codex's non-zero-exit `ErrKind` as unresolved (`ErrKindAuth` vs `ErrKindProvider`), correctly identifying that spec.json AC-03's parenthetical ("non-zero exit -> provider") contradicts its own controlling clause ("the same ErrKind mapping as the claude driver"), whose actual ratified value — verified live at `internal/driver/subprocess.go:128` — is `ErrKindAuth`. [[project_driver_contract_recut]] records this exactly: "Any future driver (S03-codex, S04-inprocess) that maps its own subprocess/API auth failures MUST reuse `ErrKindAuth`, not invent its own label — this is now the release's binding cross-driver error-kind contract." S02's own `status.json.design_decisions[0].human_decision` records Brad's 2026-07-03 ratification directly. This is not a fresh judgement call — it was already made, explicitly scoped to cover S03 by name, at S02's design review.
   What to ask the implementer: lock in `ErrKindAuth` for codex's non-zero-exit case in `spawnClassified`/`TestCodexErrorMapping` — no need to wait on a new Coach decision. Populate `status.json.design_decisions[0].human_decision` citing the S02 precedent (e.g. "Pre-ratified by Brad, 2026-07-03, S02 design review — binding cross-driver contract, applies to S03 without a fresh decision") rather than leaving it `null`.

3. [mechanical] — `spec.json` AC-03 — parenthetical text is stale and should be corrected to match the ratified contract
   What I observed: AC-03's literal text still reads "non-zero exit -> provider with stderr excerpt," which is the pre-ratification wording S02 itself carried before its own pin-2 fork flipped the value to `auth`. When that ratification landed, S06 (R-03) and S07 (R-02) each got a forward-sync risk note added to their specs — but S03's own spec.json parenthetical was not corrected in the same pass, leaving this internal contradiction for S03's implementer to discover independently (which they did, correctly, in decision 6).
   What to ask the implementer: note in journal.md that `spec.json` AC-03's parenthetical is stale and should be corrected from "provider" to "auth" via a small mechanical `/replan-release` housekeeping pass (citing this review + [[project_driver_contract_recut]]) — this does not block writing code now since design.md decision 6 already documents the correct value to build against.

## Summary

Pins: 3 total — 1 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins: #1 — codex verifier-dispatch fresh-context gap is a live Rule 7 compliance question this review cannot resolve from repo state alone.

## Smaller flags (not pins, worth one-line acknowledgement)

- (a) `sworn llm-check -type design-review` is unavailable in this environment ("no model configured") — consistent with the project's existing no-paid-dispatch posture ([[feedback_releaseverify_specmd_false_fail]] neighbourhood), not a slice-specific gap. Noting for the audit trail; does not block PROCEED.
- (b) Decision 5's claim that `subprocess.go` today hardcodes non-zero-exit -> `ErrKindAuth` in `classifySpawnError` — verified live at `internal/driver/subprocess.go:128`. The proposed `spawnClassified` split (parameterizing only the non-zero-exit arm, leaving `spawn()`'s signature and `claude.go`'s call site untouched) is accurate and correctly scoped.
- (c) Decision 4's reuse of `isJSONObject` from `claude.go` (line 102, package-scoped, no claude-specific behaviour) — verified live, correctly scoped as same-package reuse rather than duplication.
- (d) Decision 7's claim that `subprocess_test.go`'s `TestMain` already dispatches on `GO_TEST_FAKE_CLAUDE` (single `TestMain` per package constraint) — verified live at `subprocess_test.go:17-18`. Adding a parallel `GO_TEST_FAKE_CODEX` arm in the same `TestMain` is the only viable approach; correctly scoped as a touchpoint reason.
- (e) Touchpoint sequencing: S02 (verified) already landed `subprocess.go`/`subprocess_test.go` in this same track worktree, sequential before S03 — no concurrent-track collision risk; S04 (T3-inprocess, concurrent track) touches only `internal/driver/inprocess*.go`, no overlap.
- (f) Spec-completeness gate: spec.json's ACs are concrete (literal argv, field names, specific error-kind values, specific test names) — not a thin spec. No gate finding.

## Suggested acknowledgement reply

TL;DR: strong design — the implementer correctly caught and flagged spec.json AC-03's internal inconsistency rather than guessing past it, and every other factual claim (subprocess.go's current hardcoded mapping, isJSONObject reuse, shared TestMain) checked out live against the repo. One thing the implementer didn't surface that this review did: 1 pin needs your call, 1 is already answered by an existing ratified decision, 1 is a small text fix.

1. **Codex verifier dispatch needs a fresh-context answer before `TestCodexDispatchVerifier`/`TestCodexErrorMapping` lock in behaviour.** S02's own AC-03 requires `--no-session-persistence` specifically for Rule 7 (fresh-context verification); S03's AC-02 is silent on the codex equivalent, and decision 1 resolved that silence as "nothing to add" rather than as an open Rule-7 question. Confirm from codex CLI docs/help whether a bare `codex exec` call is stateless by default, or treat this as a spec gap needing a `/replan-release` amendment to AC-02. This is the one item that needs your judgement — it turns on live codex CLI behaviour, not repo state.
2. **`ErrKindAuth` is already decided — build it, don't wait.** [[project_driver_contract_recut]] and S02's `status.json` both record that codex's non-zero-exit case reuses `ErrKindAuth`, ratified at S02's design review and explicitly scoped to cover S03 by name. Lock this into `spawnClassified`/`TestCodexErrorMapping`, and fill in `status.json.design_decisions[0].human_decision` citing the S02 precedent instead of leaving it `null`.
3. **`spec.json` AC-03's "provider" parenthetical is stale text, not a design blocker.** Note it in journal.md for a small `/replan-release` housekeeping fix (provider -> auth) — build against decision 6's correct value now.

Flags (informational, no action needed): (a) llm-check unavailable in this environment, unrelated to this slice; (b)-(d) decisions 4/5/7's factual claims all verified live; (e) no touchpoint collision, S02 already sequential-landed, S04 has no file overlap; (f) spec is concrete, not thin.

§2 decisions 2, 3, 4, 5, 7 acknowledged clean. Decision 6 addressed by pin 2 (already ratified, not a fresh decision). §6/Risks-for-Captain item (R-01 restated) stands as documented — no change needed, it's an accepted, spec-mitigated risk.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: pin 1 (codex verifier-dispatch fresh-context / Rule 7 compliance) turns on live codex CLI behaviour this review cannot verify from repo state and touches a CRITICAL project rule — needs Coach judgement before TestCodexDispatchVerifier/TestCodexErrorMapping lock in behaviour. Pins 2-3 are apply-inline and would not by themselves block PROCEED.
-->
