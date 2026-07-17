# Baton v0.16.0 replan decision

## Ratified change

On 2026-07-18 the Coach directed this in-flight conformance release to adopt
Baton `v0.16.0`, whose peeled upstream tag commit is `aae82d1`. The release
identity is therefore `2026-07-15-baton-v0.16-conformance`; the date remains
the original planning-start date.

The preceding `2026-07-15-baton-v0.15-conformance` identifier is retained in
Git history only. Its release-worktree and T1 branch/worktree are renamed as
one explicit identity migration. The migration changes planning-record identity
and paths, not any source claim or verifier verdict.

## Sequencing decision

S22 stays in progress and retains its already-ratified payload-safety replan.
S20 also remains the existing v0.15.1 bootstrap: it has an immutable start and
committed implementation interval, so replacing its contract in place would
fabricate a new lifecycle. It must finish or fail through its own bounded
workflow.

Two new planned tail slices then adopt the v0.16 delta:

1. `S23-v016-parity-and-installs` vendors the exact tagged protocol, archive,
   schemas, and eight-command Codex/Claude installer surface.
2. `S24-board-oracle-v1-projection` makes Sworn's public board projection and
   reusable validator conform to `board-oracle-v1` before later role, merge,
   replan, and final-proof adapters consume it.

No provider call, credential access, source implementation, or local user-home
installation occurs in this Planner replan.

## Authoritative-state seed

Before this revision, the release assembly copied these committed T1 status
records rather than treating its stale copies as authority:

- S01 `21c52e97cd9e638ab997080918bccdc1aebd04f9`
- S04 `0c8a4802c3e78df81719d517b41b4b1f469023a5`
- S19 `7020394d91e91d8e5226975c23d2ccb8e2131207`
- S20 `82697d64744c0a50ce182a6a381a35473ca3408e`
- S22 `2c54f57f427337666db6846d5b176620eacdf2da`

The source branch was
`track/2026-07-15-baton-v0.15-conformance/T1-foundation` before the matching
identity rename. The `maintainability` objects are preserved as opaque records.
