# Journal — S03-codex-subprocess-driver

## 2026-07-06 — Design TL;DR + design review resolution

Produced `design.md` (commit `a925a05`), transitioned to `design_review`.
Flagged one Type-1 item rather than resolving it myself (Rule 9): spec.json
AC-03's non-zero-exit `ErrKind` mapping is internally inconsistent — it says
"the same ErrKind mapping as the claude driver" but then states `provider`,
while the claude driver's own ratified mapping (S02 pin 2) is `auth`.
Proposed `ErrKindAuth` as the default, left `human_decision` null in
`status.json`, and did not write `TestCodexErrorMapping`'s non-zero-exit
case pending review.

Captain design review (`review.md`, commits `8eb7867`/`5a6491b`) returned
3 pins + `DECISION: PROCEED`:

1. **[escalate] Fresh-context flag for codex verifier dispatch — RESOLVED.**
   Design had inferred "no flag needed" from spec silence rather than
   treating it as a live Rule 7 question. Brad supplied codex CLI's
   documented non-interactive-mode behaviour during review: `--ephemeral`
   avoids persisting session rollout files to disk; `codex exec resume` is
   required to continue a prior session, so a bare `codex exec` always
   starts fresh regardless. Applying `--ephemeral` for `Role==RoleVerifier`,
   mirroring `claude.go:50-51`. Design.md decision 1 updated.
2. **[memory-cited] codex non-zero-exit `ErrKind` — RESOLVED, not a fresh
   decision.** [[project_driver_contract_recut]] and S02's own
   `status.json.design_decisions[0].human_decision` already record Brad's
   2026-07-03 ratification as binding and explicitly scoped to cover
   S03/S04 by name. `ErrKindAuth` locked into `spawnClassified`/
   `TestCodexErrorMapping`. `status.json.design_decisions[0].human_decision`
   filled in citing the S02 precedent (no longer `null`).
3. **[mechanical] `spec.json` AC-03's "provider" parenthetical is stale
   text — Rule 2 deferral, not a design blocker.** Filed
   **sworn#84** as the concrete tracker (owning mechanism: a small
   `/replan-release 2026-06-28-driver-contract` housekeeping pass corrects
   the parenthetical from `provider` to `auth`). This slice builds against
   the correct, ratified value (`ErrKindAuth`) regardless of the stale spec
   text — the spec.json fix itself is out of this slice's touchpoints
   (spec editing belongs to planning) and does not block implementation.

Additional finding folded into the same review (not an original pin):
**decision 2's envelope assumption corrected.** The docs Brad supplied show
`turn.completed` carries only `usage` (`input_tokens`/`cached_input_tokens`/
`output_tokens`/`reasoning_output_tokens`) — no `model` or `duration_ms`
field. Design.md decision 2 updated; fake-binary fixture and `codex.go`'s
doc comment will encode this corrected shape rather than the originally
guessed one. `Result.ModelID`/`Result.DurationMS` fall back to the
requested model / measured wall-clock as the **normal** path for codex, not
a rare edge case — AC-04's required behaviour is unchanged.

Design-review resolution applied to `design.md` + `status.json` in commit
(this session, prior to "start implementation"). Proceeding to
`in_progress`.

## Next

Implement per the amended design.md: `subprocess.go` split
(`spawnClassified`), `codex.go` (`CodexDriver`), `subprocess_test.go`
(`GO_TEST_FAKE_CODEX` fixtures in the shared `TestMain`), `codex_test.go`
(AC-01..AC-05 coverage). `claude.go`/`claude_test.go` stay untouched.
