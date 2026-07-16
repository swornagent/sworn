# Fail-closed rollback checks

All objects below were created with Git plumbing only, in disposable detached
worktrees off repair checkpoint `c6842d8db3404843fdc8441a8cfefa41c03bd917`.
Each worktree was removed after its run; no release ref changed. The required
live command first passed with the 45-path envelope, amendment schema/record,
exact two-transition spec history, and detached deterministic index render.

```text
CASE arbitrary_index_render_drift: rejected exit=1
ROLLBACK_CHECK FAIL: committed release index does not byte-match the disposable current-head sworn render

CASE arbitrary_post_start_spec_blob: rejected exit=1
ROLLBACK_CHECK FAIL: unrecognized ordinary S19 spec transition c2928458fa954e8b073dd704834a09558576752b

CASE amendment_record_schema_tamper: rejected exit=1
ROLLBACK_CHECK FAIL: contract amendment record blob differs from the planner-ratified record

CASE mode_or_blob_drift: rejected exit=1
ROLLBACK_CHECK FAIL: unexpected later ordinary authority 4b38887e666f7e4ab664bac4780535b080ad54eb; only the pinned implementation head may restore semantics

CASE surviving_s02_added_path: rejected exit=1
ROLLBACK_CHECK FAIL: unexpected later ordinary authority 4b38887e666f7e4ab664bac4780535b080ad54eb; only the pinned implementation head may restore semantics

CASE unexpected_later_ordinary_authority: rejected exit=1
ROLLBACK_CHECK FAIL: unexpected later ordinary authority 4b38887e666f7e4ab664bac4780535b080ad54eb; only the pinned implementation head may restore semantics

CASE unrecognized_merge_or_parent_two_drift: rejected exit=1
ROLLBACK_CHECK FAIL: unexpected later ordinary authority 4b38887e666f7e4ab664bac4780535b080ad54eb; only the pinned implementation head may restore semantics

CASE authored_merge_overlap: rejected exit=1
ROLLBACK_CHECK FAIL: unexpected later ordinary authority 4b38887e666f7e4ab664bac4780535b080ad54eb; only the pinned implementation head may restore semantics

CASE weakened_s20_gate: rejected exit=1
ROLLBACK_CHECK FAIL: non-S19 or non-lifecycle release record changed after S19 start: docs/release/2026-07-15-baton-v0.15-conformance/S20-v015-parity-portable-fixture/status.json

CASE absent_fresh_verifier_evidence: rejected exit=1
ROLLBACK_CHECK FAIL: fresh verifier evidence is absent or not bound to the implementation head

ADVERSARIAL_OBJECTS PASS: all disposable cases rejected; release refs unchanged
```

The three new contract-bound cases exercise their intended guards directly:
wrong index bytes fail the isolated render comparison, wrong spec bytes fail the
exact transition history, and a changed amendment record fails its pinned
schema/record identity. The semantic provenance objects are also rejected by
the original pinned-head authority gate before they can narrow the envelope.
S20 remains fail-closed: a successor state is permissible only after the
exact-head Implementer PASS/report binding, a complete proof bundle, and a
fresh `state: verified` PASS with timestamp.
