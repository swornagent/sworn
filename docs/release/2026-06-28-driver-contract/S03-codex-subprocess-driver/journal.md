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

## 2026-07-06 — Implementation

Built per the amended design.md (commit `3c25d36`, start_commit `71b4e5c`):
`subprocess.go`'s `spawn`/`classifySpawnError` split into `spawnClassified`
with a parameterized non-zero-exit `ErrKind`; `codex.go` (`CodexDriver`,
`codex exec --json -C <dir>` + `--ephemeral` for verifier, JSONL envelope
parsing per the corrected shape); `subprocess_test.go`'s shared `TestMain`
extended with a `GO_TEST_FAKE_CODEX` arm + `fakeCodex*` fixtures;
`codex_test.go` covering AC-01..AC-05 (mirrors `claude_test.go`'s shape).
`claude.go`/`claude_test.go` untouched, as designed.

`go build ./...`, `go vet ./...`, `go test ./internal/driver/... -v` (30
tests, all PASS — S02's `TestClaude*`/`TestSpawn_*` unmodified and still
green), and `go test ./...` (full suite, no regressions) all green.

**Proof-bundle verification gate — environment limitation, documented
rather than worked around.** `sworn verify --spec ... --diff ... --proof
...` (the model-backed reference gate) errors `model:
SWORN_ANTHROPIC_API_KEY not set` — this environment has no model key
configured, consistent with the project's existing no-paid-dispatch posture
(already flagged as a non-blocking finding in this same slice's
`review.md`, flag (a): `sworn llm-check` is unavailable for the same
reason). Ran `~/.claude/bin/release-verify.sh S03-codex-subprocess-driver
2026-06-28-driver-contract` instead (the deterministic, model-free
first-pass half of Rule 7). Result: `spec.md missing` / `proof.md missing`
FAIL — both are the documented false-negative pattern for this project's
JSON-spec convention ([[feedback_releaseverify_specmd_false_fail]]: this
project uses `spec.json`/`proof.json`, not the generic template's
`spec.md`/`proof.md`; S02's own verified sibling slice has no `spec.md`
either). `state is in_progress` FAIL is expected at the point the script
ran (before this session's own `implemented` transition). Every
substantive, non-false-negative check passed: `status.json` valid JSON,
5 files changed vs. `start_commit` matching `actual_files`, no dark-code
markers in changed source files. The script's own tail crashes on an
unrelated pre-existing bug (`PLAYWRIGHT_OPTIN: unbound variable`) after all
relevant checks for this slice had already run and reported.

Net: no model-backed gate could run in this environment; the deterministic
half is green modulo known false negatives. Not claiming a fabricated
"first-pass PASS" from a gate that didn't run — surfacing this plainly to
the human and to the fresh-context verifier.

State: `implemented`. Stopping here — per role boundaries, this session
does not run a verifier prompt or claim `verified`.

## Next

`/verify-slice S03-codex-subprocess-driver 2026-06-28-driver-contract` in a
fresh terminal session (Rule 7 — no inherited context from this session).
