# External approval

Brad approved this exact amended revision 2 in this conversation on
2026-09-28 by replying "LFG !" to the message that stated its plan digest, that
S1 to S5 keep their revision 1 contract files and digests, the five repair
slices S6 to S10 with the defect each repairs, the -parallel=8 e2e host check
for the repair slices, and the launch steps (rebase onto the release head,
record, run r3 with the same roster). The same reply also approved a separate,
pruned list of engine issues, which is not part of this plan. This is an actual
user response, not an inference.

This revision supersedes the revision 2 approved on 2026-09-26 for plan
972df6a0... (kept as APPROVAL-972df6a0.md), which was never recorded.

- Release: 2026-09-25-recovery-robustness
- Plan revision: 2
- Previous plan: 9b78a34f624001ddd33b6715997b7ee3eb0aaa56 (revision 1)
- Plan SHA-256: 6b3ae2f65aee69085e488e72478d9cd90f692a90ab7c3dfdaf4f76a20b67134e
- Approval reference: operator://2026-09-25-recovery-robustness/2
- Release base: release/2026-09-25-recovery-robustness at 1fa5d724 (S1 to S5
  verified and merged by run r2)
- Repair slices: S6-host-environment-park-projection,
  S7-pause-safe-host-checks-repair, S8-credential-lifetime-repair,
  S9-broker-budget-and-turn-cap-repair, S10-repair-input-across-epochs-repair
- Roster: planner and lead claude/claude-opus-5-5; verifier
  claude/claude-fable-5-1; implementer claude/claude-sonnet-5;
  automation.recovery claude/claude-haiku-4-5

Approval authorizes the ten slices as written; S1 to S5 are unchanged from
revision 1 and already verified. The approved plan bytes retain their proposal
wording; this document records the subsequent approval without altering them.
Native recording binds the actual prepared target and contract-tree commit
before dispatch. No candidate PASS or merge to main is implied. The run is
driven by the Manager seat under docs/policy/manager.md version 3.
