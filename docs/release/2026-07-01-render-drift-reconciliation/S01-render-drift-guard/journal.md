# Journal — S01-render-drift-guard

## 2026-07-02 — Implementer session start

Design review (`review.md`, commit `a1ad9bf`) returned `DECISION: PROCEED`,
`CONSTITUTIONAL: no`, 4 pins (2 mechanical, 2 memory-cited, 0 escalate). No
`approved-ack.md` marker existed on disk, so per Rule 9 (design stays
human-owned) I asked the Coach (Brad) directly whether he acknowledged the
verdict before starting implementation. He confirmed: "Acknowledge, proceed."
Recording that ack here since it's the durable artefact — this note is the ack
record.

Applying the 4 pins inline during implementation:
1. Populated `status.json.planned_files` (4 touchpoints) and
   `design_decisions` (byte-for-byte comparison; full driftGuard removal) —
   both Type-2, both `architecturally_significant: false`. Re-ran
   `sworn designfit 2026-07-01-render-drift-reconciliation` after populating —
   see result below.
2. AC-05 proof step will capture `checkRenderDrift`'s own OK/ERROR lines, not
   chase `sworn doctor`'s overall exit code (pre-existing non-zero from 95
   unrelated status-timestamp violations in other releases).
3. Will note the `spec.md missing` `release-verify.sh` false-negative in the
   proof bundle's first-pass section (spec-v1/spec.json slice, no spec.md by
   design — see `feedback_releaseverify_specmd_false_fail` memory).
4. Will run full `go test ./...` before claiming the proof bundle done, not
   just the AC-06-scoped `./internal/board/... ./cmd/sworn/...`.

## 2026-07-02 — Implementation complete

Implemented `checkRenderDrift` (`cmd/sworn/doctor.go`), wired into
`cmdDoctor`'s Group 2 feeding `hasError` (AC-02). Removed
`internal/board.driftGuard` and its `WriteBoard` call site + the now-unused
`log` import entirely (AC-04). Added 4 new tests
(`TestDoctorRenderDrift_{Clean,Drifted,NoBoardSkipped,RenderError}`) using
`t.TempDir()` fixtures built via `writeRenderDriftFixture`.

Discovered mid-session: this release's own `docs/release/.../index.md` had
drifted (S01's own state transitions weren't reflected — last rendered
before design review). Re-rendered it via `sworn render` — a one-line
state-column diff — required for AC-05's live-repo proof step to hold
(pin 2). Also discovered `TestDoctorAllOK`'s repo-root resolution is off by
one path segment and has never actually exercised Group 2/2b against this
repo's real `docs/release/` content — pre-existing, unrelated to this
slice, filed as swornagent/sworn#49 (Rule 3) rather than fixed here (Rule 2:
out of scope, tracked, acknowledged in this journal).

`sworn coverage` and `sworn llm-check --check ac-satisfaction` (cited in the
implementer role prompt as reference gates) do not exist in this repo's
vendored `sworn` binary — confirmed via `sworn --help`. Proceeded without
them; relied on `sworn designfit`, `go test ./...`, and the real `sworn
doctor` run instead. Not a slice-scoped gap — the binary's command surface
doesn't match the role prompt's reference implementations for these two
gates specifically.

Full `go test ./...` (38 packages) passes — no regression from the shared
`WriteBoard` change (pin 4). `proof.json` and the updated `status.json`
both validate against `proof-v1` / `slice-status-v1` (checked via a
temporary in-module `baton.Validate` test, removed after use).

State -> `implemented`. Stopping here per role boundaries — no verifier
prompt in this session.

## 2026-07-02 — First-pass gate (release-verify.sh)

Ran `~/.claude/bin/release-verify.sh S01-render-drift-guard
2026-07-01-render-drift-reconciliation` (private harness tooling, not part
of this repo). Result: every check before the crash point PASSED (slice
folder, status.json valid JSON + state=implemented, diff-vs-start_commit
8-file list, no dark-code markers, frontmatter YAML safety). Two expected
FAILs (`spec.md missing`, `proof.md missing`) — both known false negatives
for spec-v1/proof-v1 (JSON) slices; confirmed no verified sibling slice in
this repo's current-schema releases carries either file. The script then
crashed on an unbound `$PLAYWRIGHT_OPTIN` (only ever set inside the
proof.md-gated block) before printing its final verdict line — a private-
harness bug, not a sworn-repo issue, flagged to the Coach at session end
rather than filed as a GitHub issue. Full detail recorded in proof.json's
divergence section. Treating first-pass as green given every substantive,
applicable check passed.

## Verifier verdicts received

### 2026-07-02 — FAIL

Fresh-context verifier session (Rule 7), no inherited implementer context.
Discovered inside track worktree at
`/home/user/projects/sworn-worktrees/release-2026-07-01-render-drift-reconciliation-T1-drift-guard`
via the release board oracle (`sworn board --release
2026-07-01-render-drift-reconciliation --json`) + `git worktree list`; zero
drift against `release-wt/2026-07-01-render-drift-reconciliation` at time of
verification (no forward-merge needed).

**Verdict: FAIL.** 3 violations. Full detail in the FAIL block returned to
the human this session; summary:

1. Gate 3b (AC-05 not satisfied) — rebuilt `sworn` from this worktree's HEAD
   (`3679145`) and ran `sworn doctor` for real. It reports
   `[ERROR] render drift (2026-07-01-render-drift-reconciliation)` — this
   release's own committed `index.md` does not match `board.Render`'s
   output, because commit `b96f27a` set S01's `status.json.state` to
   `implemented` without re-rendering `index.md` (last rendered in
   `f0792d2`, while S01 was still `in_progress` — see this journal's own
   "Implementation complete" entry, which only mentions rendering once,
   mid-session). AC-05 requires zero drift errors for T1-T5-scope releases
   beyond the two named tracked exceptions (#44, #45); this release itself,
   in scope, is not one of them.
2. Gate 4 / Gate 7 (stale reachability artefact / delivered claim) —
   `reachability-doctor-output.txt` and `proof.json`'s AC-05 `delivered`
   entry both claim "zero render-drift errors beyond the two tracked
   exceptions," captured in the same commit (`b96f27a`) that introduced the
   drift described above. A fresh run at HEAD reports 3 drifted releases,
   not 2.
3. Gate 2 (minor, undocumented touchpoint mismatch) — `spec.json` lists
   `internal/board/board_test.go` as a planned touchpoint; it was never
   modified in this slice's diff, and neither `proof.json`'s `not_delivered`
   (empty) nor its `divergence` section explains the omission.

Confirmed via "Before you FAIL" gate: all three are legal implementer fixes
(re-render `index.md` + re-capture the reachability artefact after the
final `status.json` state settles; add one documentation sentence for
violation 3) — none require a different test shape, touch an accepted
deferral, or need planner authority. Verdict is FAIL, not BLOCKED.

`state` -> `failed_verification`. Re-opens via `/implement-slice
S01-render-drift-guard 2026-07-01-render-drift-reconciliation` in a fresh
session.

## 2026-07-02 — Re-entry after FAIL (implementer)

Picked up per Step 0b: `verification.result` was `"fail"`, not `"blocked"` —
normal `failed_verification` re-entry, no `/replan-release` routing needed.
`start_commit` (`ae390d1f...`) and the historical FAIL `verification` block
were left untouched on the `in_progress` transition (S02b lesson / cycle-2
precedent from S25-event-store-durable: never null `start_commit` or the
prior verdict at re-entry — cycle-1 of that slice did, and it was wrong).

All three violations were process/documentation, not code defects — no
production code changed this round:

1. **Gate 3b (render drift).** Confirmed the root cause directly: rendered
   `index.md` at the current HEAD (state `failed_verification`) and diffed
   against the committed file — zero diff, so the verifier's own FAIL
   commit had already incidentally fixed the drift by re-rendering when it
   recorded the verdict. The actual fix is procedural: re-render `index.md`
   in the *same commit* as every `status.json` state change, never as a
   separate follow-up. Did this for both transitions this session
   (`in_progress`, then `implemented`) — confirmed via `git diff --stat` on
   `index.md` each time (one-line state-column diff, nothing else moved).
2. **Gate 4/7 (stale artefact).** Recaptured `reachability-doctor-output.txt`
   from a fresh `sworn doctor` run against the working tree *after* the
   final `index.md` re-render, so the artefact reflects the exact file
   state that lands in the commit — not a state captured before a
   subsequent change. Confirmed: render-drift errors only for the two
   AC-05-tracked exceptions (#44, #45); zero for this release.
3. **Gate 2 (touchpoint mismatch).** Added one sentence to `proof.json`'s
   `divergence`: `internal/board/board_test.go` was never modified because
   `driftGuard` had no existing board-package test to remove — grepped its
   history to confirm — so there was no test surface this deletion touched.
   The new behavior is tested where it lives (`cmd/sworn/doctor_test.go`).

Re-ran `go build ./...`, `go test ./internal/board/... ./cmd/sworn/...`,
and full `go test ./...` (38 packages) — all green, no regressions.

First-pass gate: `sworn verify` (model-backed) requires
`SWORN_ANTHROPIC_API_KEY`, unset in this environment — same gap as any
session without that key, not new this round. Ran
`~/.claude/bin/release-verify.sh` (private harness) instead, anchored at
this worktree: state=implemented, 8 files changed vs `start_commit` (matches
`proof.json`'s `files_changed`), no dark-code markers, both known
spec.md/proof.md false negatives (per `feedback_releaseverify_specmd_false_fail`),
crashes on the same pre-existing unbound `$PLAYWRIGHT_OPTIN` after every
applicable check passed. First-pass green.

`state` -> `implemented`. Stopping here per role boundaries — no verifier
prompt in this session.

### 2026-07-01 — PASS

Fresh-context verifier session (Rule 7), no inherited implementer context.
Discovered inside track worktree at
`/home/user/projects/sworn-worktrees/release-2026-07-01-render-drift-reconciliation-T1-drift-guard`
via the release board oracle (`sworn board --release
2026-07-01-render-drift-reconciliation --json`) + `git worktree list`; worktree
clean, zero drift against `release-wt/2026-07-01-render-drift-reconciliation`
(no forward-merge needed). Prior recorded verdict was FAIL
(2026-07-02T00:00:00Z) but `status.json.state` carried a later
`implemented` timestamp (2026-07-02T00:30:00Z) — a re-entry round had already
landed, so all six gates were re-run fresh rather than short-circuiting (the
idempotent-BLOCKED shortcut only applies to `result: "blocked"`).

**Verdict: PASS.** All six gates satisfied:

1. **Reachable outcome** — `checkRenderDrift` (`cmd/sworn/doctor.go`) is wired
   directly into `cmdDoctor`, the real `sworn doctor` CLI entry point (not a
   leaf-only path). Confirmed by building the binary from this worktree's
   HEAD and running `sworn doctor` for real.
2. **Touchpoints** — `internal/board/board_test.go` was planned but not
   touched; `proof.json`'s divergence section explains why (no prior
   board-package test covered `driftGuard`, nothing to remove) — the
   omission the prior FAIL flagged as undocumented is now documented.
3. **Tests** — re-ran `go build ./...`, `go test ./internal/board/...
   ./cmd/sworn/...` (both green), and full `go test ./...` (all 38 packages
   green). `TestDoctorRenderDrift_{Clean,Drifted,NoBoardSkipped,RenderError}`
   in `doctor_test.go` exercise `cmdDoctor` end-to-end via `runDoctorInDir`,
   not the leaf function in isolation.
4. **Reachability artefact** — rebuilt `sworn` from this worktree's HEAD and
   ran `sworn doctor` for real (not a fixture): render-drift reports exactly
   2 errors, both AC-05-tracked exceptions (`swornagent/sworn#44`,
   `swornagent/sworn#45`), zero for this release or any T1-T5-scope release.
   Matches `reachability-doctor-output.txt` byte for byte. The drift the
   prior FAIL caught (stale `index.md` at the `implemented` transition) is
   gone — this round re-rendered it in the same commit as the state change.
5. **No silent deferrals** — grepped `doctor.go`, `doctor_test.go`,
   `board.go` for TODO/FIXME/deferred/placeholder/stub: no hits.
6. **Scope** — each `delivered` entry in `proof.json` cites a specific test
   or artefact; spot-checked AC-01/02/03/04/05/06 against the live diff and
   live `sworn doctor` run, all consistent.

`state` -> `verified`. Track `T1-drift-guard` has only this one slice —
track is now complete.

**Next step:** `/merge-track T1-drift-guard 2026-07-01-render-drift-reconciliation`,
then `/merge-release 2026-07-01-render-drift-reconciliation` once every other
track (T2-T5) has also merged.
