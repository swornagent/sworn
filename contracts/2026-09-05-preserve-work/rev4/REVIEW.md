# Revision 4 proposal validation

Status: proposed, not externally approved or natively recorded.
Plan SHA-256: 309359d1c252287b4b1d38dadc369b3010a1d600fbc83256f64785299fddbd31.
Previous approved plan blob: 27379f92b9f6f1d33a196084e19f8f72bae3974d (revision 3).

What changes against revision 3:

- S2 contract only: three constraints appended (stale checkpoints as
  reference-only; contained-workspace facts that wasted turns; the observed
  crash-cut recovery seam). Outcome, scope, acceptance, checks, host_checks,
  depends_on and consumes are byte-identical to revision 3. New S2 digest
  sha256:5a92ba01d576f2c980a4256744ef4153ac8b358639bc75de6b6b4a4a217024dd.
- S1, S3, S4: revision-3 contract paths and bytes, digests unchanged
  (c6f8cfe5..., 73c72eff..., 60b47bbb...). S1's verified pass carries.
- plan.md prose: records the lane change (implementer claude/claude-sonnet-5,
  verifier claude/claude-opus-5, captain claude/claude-opus-5), the 2-hour
  per-invocation timeout, and that the revision-3 run stays parked and inert.
- No product source, CI, AGENTS.md or recovery input changes. Planning base
  is the current approved target 0021acc7.

Native pin is idempotent; native scope lint PASSes all four slices with the
validated ops/sworn-repair binary (364f0b74...). Doctor results for the new
roster are recorded in the ops operator log.

Evidence for the change: run 2026-09-05-preserve-work-recovery, S2
implementation tries e1/t1 (CONTINUATION_INVALID, then scope fence),
e2/t1, e2/t2, e2/t3 (INVOCATION_TIMEOUT at 60 min each), checkpoints
09b237b5, 3965416d, a2c43228; net convergence about 27 lines/hour.
Filed: sworn#291 (Gemini correlate defect), sworn#292 (role session
continuity ruling).

Before native recording: obtain actual external approval of the exact plan
digest above, write rev4/APPROVAL.md, commit the preparation files, fast-
forward the release target to that commit, and record with --commit and
--contract-tree bound to it.
