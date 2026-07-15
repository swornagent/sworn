# Journal — S01-vendor-boundary-readiness

## 2026-07-16 — Implementer design checkpoint

- Transitioned `planned -> design_review`; no production code or tests were
  written.
- Bootstrap oracle adapter explicitly authorised by the planning authority:
  invoked the installed pre-cutover read-only oracle as
  `sworn board --release 2026-07-15-baton-v0.15-conformance --json`, adapted its
  legacy top-level `{release, tracks}` result, and resolved S01 as the first,
  dependency-ready slice in `T1-foundation`.
- Governing role source adapter: used the clean Baton `v0.15.1` checkout at
  `/home/brad/projects/baton`, commit
  `3fb4d275ae8a151f6287e7b9279d71628b12eea0`, specifically
  `commands/implement-slice.md`, `baton/track-mode.md`, and
  `baton/role-prompts/implementer.md`, because the installed role docs remain
  pre-cutover until S02.
- Materialised the conventional release and T1 worktrees from clean
  `release/v0.2.0` commit `135a01e1c4e0e2825a40ddd93618c3cbc906fdea`.
- Readiness evidence: pinned v0.15.1 `spec-v1` and `slice-status-v1` validation
  passed; status history preserves null `start_commit` and the empty pending
  cycle-0 maintainability record; `sworn lint ac`, `sworn lint trace`,
  `sworn reqvalidate`, and `sworn designfit` all passed.
- Design-review pin: AC-03/AC-04 require CLI exit changes in
  `cmd/sworn/baton.go`, but that file is absent from the slice touchpoints. The
  Captain must require a planner correction or decline before implementation;
  the Implementer will not cross that ownership boundary.
## 2026-07-16 — Coach-ratified narrow replan

- Seeded the authoritative lifecycle from owner ref
  `track/2026-07-15-baton-v0.15-conformance/T1-foundation` at Captain review tip
  `1bc4d7508960d83182e2177a18374df530c632fc`; source `status.json` blob
  `fe20a546c5b9b98e42a245ac4933a267047e27c9`. The `design_review` state and
  complete pending cycle-0 `maintainability` object are preserved.
- Captain review commit `1bc4d7508960d83182e2177a18374df530c632fc`
  returned `NEEDS_COACH`: S01 could not own the public exit map or durable
  recovery authority, and the upstream pin sat outside the mapped-file
  transaction.
- Coach decision: add `cmd/sworn/baton.go`, `internal/baton/version.go`, and
  `internal/baton/version_test.go`; bind public vendor exits 0/1/2; construct
  upstream VERSION replacement bytes before mutation from one captured
  invocation instant and include them in the mapped-file transaction; and make
  the fixed Git-admin-confined owner-only recovery record the sole,
  integrity-checked recovery authority. Excluding pin writes from the atomic
  boundary is rejected because it preserves partial success.
- S01 changes transaction machinery and tests only. It does not execute the
  v0.15.1 pin/content/install update; S02 retains that outcome.
- The expanded design must be revised and receive a fresh Captain review before
  implementation resumes; no production code was authorized here.

## 2026-07-16 — Implementer design revision after Coach resolution

- Revised `design.md` against Coach-ratified replan
  `05eefeb0c849b22a68f669a80de199ac071c023f` and the exact v0.15.1 role source
  at `3fb4d275ae8a151f6287e7b9279d71628b12eea0`; no production code, tests,
  proof, acknowledgement, or lifecycle transition was created.
- Resolved Captain pin 1 in the design: S01 now plans `cmd/sworn/baton.go`,
  `internal/baton/version.go`, and `internal/baton/version_test.go`; the public
  command captures one invocation instant, pure VERSION bytes join the complete
  transaction before mutation, and exits are exactly 0/1/2.
- Resolved Captain pin 2 in the design: the sole restart authority is the fixed,
  owner-only Git-admin sentinel plus a non-self-referential manifest-addressed
  transaction; a fresh invocation validates the complete confined material set
  and rejects tamper, traversal, missing, symlinked, mode-drifted, duplicate, or
  foreign material before any destination write.
- Preserved Captain pin 3 and the ownership boundary: the exact normative schema
  bytes remain compiler input, while S02 still owns executing the v0.15.1
  content/pin update and Codex/Claude mirror installs.
- Coach resolution is incorporated, but not acknowledged as design approval.
  State remains `design_review`; a fresh Captain review is required before any
  `in_progress` transition or implementation.
