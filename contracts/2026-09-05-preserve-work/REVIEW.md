# Proposal validation

Release: `2026-09-05-preserve-work`, revision 1. This is a local planning
proposal, not an approval, implementation receipt, or completed release.

Manifest: `plan.md`

- SHA-256: `8d67b13bbee0744acfcbe4b7a506c5b50537d7ea165258109ce0173404961aa4`
- Git blob identity: `cf6bb6e45599c7f4e45d57d762d6feb947c52abc`
- Inspection/source-tool baseline: `5a9aefeced602114f0dee25fc3d2b370651c82cf`
- Planning branch: `plan/2026-09-05-preserve-work`
- Proposed integration branch: `release/2026-09-05-preserve-work`

## Validation performed

Sworn's current source CLI pinned all four contract digests and passed all
four native scope-lint results. A second pin produced byte-identical output.
The commands were `GOFLAGS=-buildvcs=false go run ./cmd/sworn plan pin` and
`plan lint`, with this worktree and its absolute manifest path.

The installed `/home/brad/go/bin/sworn` reports `1.0.0-rc.2-dev` but lacks
the `plan` command; it was not used as the planning validator. No installed
binary was replaced.

An independent read-only content reviewer examined the actual loss paths and
the contracts. The first review identified missing protection for the sole
workspace on failed capture, unspecified storage retention, incomplete native
budget coverage, and unknowable post-crash usage. Those points and the smaller
scope/legacy corrections were incorporated. The final content review reported
no remaining blockers. It was not an engine approval or verification verdict.

No production code was changed. No live run, approval record, candidate,
release target or existing track was mutated. The long product and host test
suites have not been rerun for this proposal, and no commit has been made.

## Next authorized transition

Present the exact manifest and its pinned contracts to Brad. Sworn's embedded
planner operation requires external approval of the exact proposed bytes;
agreement with the earlier programme summary is not that recorded approval.

After approval and integration of the existing foreign-repository release,
prepare the proposed release branch from the verified integrated main,
recheck this plan against that actual base, run the applicable repository
checks before committing the contracts, and record the approved plan through
Sworn's native command surface. Preserve the plan bytes and digests; if a
promised outcome changes during preparation, present a forward-only revision.

Implementation and independent verification then proceed through Sworn's
embedded role contracts. Do not invoke the retired standalone Baton skills.
