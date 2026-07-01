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
