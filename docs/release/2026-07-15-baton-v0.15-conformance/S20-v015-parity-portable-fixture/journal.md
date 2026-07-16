# S20-v015-parity-portable-fixture journal

## 2026-07-16T22:53:25+10:00 — Planned

- Fresh C-01 owner assigned after mandatory S19 rollback.
- Re-delivers the exact 45-path S02 semantic result under fresh lifecycle authority.
- Corrects Gate 3 by verifying and cloning the pinned exact-tag Git bundle beneath `t.TempDir()`; no synthetic commit, weakened production predicate, or developer sibling checkout is permitted.
- Real Codex and Claude homes remain untouched until this slice is independently verified.

## 2026-07-17T04:06:59+10:00 — Design TL;DR produced

- Entered design_review; start_commit remains unset and no S20 source, vendor,
  archive, bundle, fixture, or installation bytes have been changed.
- Confirmed S19's fresh PASS and the live 45-path frozen semantic envelope. The
  proposed clean-CI source oracle is the authenticated committed v0.15.1 Git
  bundle, cloned only beneath t.TempDir() after byte/blob/tag/commit validation;
  it replaces the historical ambient sibling checkout without becoming a runtime
  dependency.
- The design keeps mapped bytes, the separate adopting VERSION manifest, and
  the complete 78-entry installer archive in one repository transaction; it
  keeps all three isolated install roots under one pre-published recovery
  authority and leaves broader state/lifecycle semantics to their owning slices.
- All declared implementation touchpoints are T1-only. Awaiting a fresh Captain
  PROCEED decision before implementation.

## 2026-07-17T04:26:26+10:00 — Automatic Coach acknowledgement and Captain PROCEED

- Under the Coach's standing instruction to orchestrate this release, the
  Captain's `PROCEED` verdict in `review.md` (commit `57157f8`) is
  acknowledged. There are no `[escalate]` pins and no new Type-1 decision to
  seek.
- Apply pin 1 inline: preserve the public built-binary `baton diff` contract
  with exact clean `0`, deterministic drift `1`, and malformed or missing
  source `2` coverage, without mutation.
- Apply pin 2 inline: keep `doctor --sync-baton` on the stdlib-only
  installation and recovery path; prove isolated sync works with Go and a
  shell unavailable on `PATH` after test-only setup.
- Apply pin 3 inline: run every installer oracle and built-binary proof in
  explicit, contained, pairwise-disjoint temporary roots, and fail if a real
  home can be selected.
- Apply pin 4 inline: treat the ambiguity-schema source mapping, embed,
  fixture, and manifest as one byte-exact authority set. Compare the full
  archive inventory directly against the verified temporary clone.
- Proceed to `in_progress` only in a fresh Implementer session. That session
  must stop at `implemented`; fresh adversarial verification remains the
  certification backstop, and no real local Codex or Claude installation is
  authorised yet.

## 2026-07-17T05:03:46+10:00 — Implementation checkpoint

- Bound the immutable S20 start anchor to
  `08dd38f81e466d3288ff4bf64953cfc90ea6063c`; all S20 evidence and the later
  maintainability review use that commit as their base.
- Reconstructed the frozen 45-path semantic result and added the committed
  authenticated v0.15.1 Git-bundle fixture. The clean-CI oracle validates the
  bundle's size, SHA-256, Git blob, v2 header, complete history, annotated tag,
  peeled commit, root VERSION object/bytes, clean status, and fsck before the
  built public `baton diff` receives its temporary checkout.
- Applied all Captain pins: the public adapter has explicit 0/1/2 behavior;
  `doctor --sync-baton` bypasses external dependency checks; child-process
  proof roots are explicit, contained, pairwise-disjoint temporary homes; and
  the new ambiguity schema plus full installer archive inventory are compared
  byte-for-byte to the verified clone.
- No real Codex or Claude home was selected or mutated. The native sync proof
  used disposable roots with `PATH` containing no shell or Go executable and
  observed exits 2 then 0.

## 2026-07-17T05:12:00+10:00 — AC-satisfaction remediation

- The configured AC-satisfaction check surfaced a real AC-04 adapter defect:
  a completed `InstallRecovered` restoration returned public exit 2, while the
  contract reserves exit 1 for verified restoration that still requires an
  explicit later install.
- Added `TestDoctorSyncBatonRecoveryExitIsOneBinaryReachability` first. It
  created a durable recovery record through the test-only transaction seam,
  ran the built binary in contained no-PATH-tool roots, and failed with
  `exit = 2, want 1`.
- The thin `cmdDoctorSyncBaton` adapter now maps `InstallRecovered` to 1;
  repair remains 2 and already-exact remains 0. The new built-binary test and
  the complete repository suite pass after the repair.

## 2026-07-17T05:19:45+10:00 — Blocked: generic check identity contract

- After the AC-04 recovery-exit repair, the configured exact
  `ac-satisfaction` operation emitted `{\"verdict\":\"PASS\",\"findings\":[]}`.
  The upgraded `llm-check-report-v1` schema requires `check`, so the operation
  failed closed despite the model's stated PASS. The exact prompt asks only for
  `verdict` and `findings`, which is the contract mismatch.
- S04 AC-04 exclusively owns the canonical requested/emitted generic-check
  identity contract. S20 AC-05 explicitly excludes requested-check matching,
  so this slice cannot correct the mismatch without widening its scope.
- Preserve immutable start `08dd38f81e466d3288ff4bf64953cfc90ea6063c` and
  committed implementation heads `edad0fa8a75ab3b4a1938bdaf856c7973be72107`
  and `f3da6a49c3f89f0883e265befd30d1eb099d6a90`; no product, proof, or
  workaround change was made in response to this blocker.
- Tracking: `/replan-release 2026-07-15-baton-v0.15-conformance`. The Coach
  directed this block and explicitly prohibited S20 scope widening or a
  synthetic report workaround.

## 2026-07-17T05:28:20+10:00 - Planner replan: S04 prerequisite

- T1 now orders `S04-typed-reference-ambiguity` immediately before S20.
- S04's existing AC-04 is the sole owner of requested/emitted generic-check
  identity. S20 AC-05 remains explicitly excluded from that behavior; no S20
  implementation, proof, workaround, or scope expansion is authorized.
- S20 remains `blocked`; its immutable start
  `08dd38f81e466d3288ff4bf64953cfc90ea6063c` and existing implementation
  evidence remain authoritative.
- Resume S20 only after a fresh verifier PASS for S04, then rerun S20 readiness
  and maintainability evidence before any S20 finalization.
