# Design TL;DR — S19-s02-v015-rollback

**Slice:** `S19-s02-v015-rollback` · **Track:** `T1-foundation` · **Release:** `2026-07-15-baton-v0.15-conformance`
**State:** `design_review` — no semantic rollback changes have been made
**Covers:** `N-07`, `N-09` · **Effort/complexity:** high / low (`grind`)

## User outcome

A release operator can prove that every ordinary S02-authored semantic path is
back at the exact S02 start-tree identity before a fresh replacement slice is
allowed to introduce v0.15.1 behaviour. The failed S02 records remain intact;
this is a semantic restoration, never a history rewrite.

## Approach

1. **Define the authority boundary from immutable Git identities.** Use S02's
   immutable start commit `e61cb190736ee7483fb4ed1a993442b26ce3574c` (tree
   `c57285e3f652e5f49aa8bb15e3ba65249b4a3db8`) as the only semantic baseline.
   A proof-side checker will walk every first-parent, non-merge commit from that
   start through S19's pinned `maintainability.implementation_head`; it will
   collect every changed path outside the physical release root. The current
   45-path list is a review control, not a narrowing allow-list: an added,
   generated, lock, gap, or later rollback-authored semantic path is included.
2. **Restore paths, not commits.** For every baseline-present envelope path,
   restore the exact start-tree blob and mode. Remove every baseline-absent
   envelope path. Do not use `git revert`, cherry-pick, reset, or a broad tree
   checkout: those operations would risk overwriting the append-only S02 claim,
   receipt, reports, proof, journal, verifier failure, and terminal deferral.
3. **Make equality fail closed.** The committed proof checker will compare the
   final envelope to the start tree path-by-path (mode, blob OID, and absence),
   reject semantic changes on an unrecognised merge, reject overlap between an
   authored and merge contribution, and reject any ordinary authority outside
   the reconstructed envelope. A whole-tree `git diff --exit-code` excluding
   only `docs/release/2026-07-15-baton-v0.15-conformance/**` is the independent
   backstop against an omitted path.
4. **Preserve release evidence deliberately.** The checker will also establish
   that S19 changed no prior release records: changes under the physical release
   root after S19's start commit may be only S19's own design, review, proof,
   status, journal, and maintainability records. Existing S02 records must keep
   their exact blobs. This makes the normal S19 lifecycle artefacts the sole
   release-root exception.
5. **Prove the operational state before handoff.** Capture the complete
   envelope inventory, equality result, negative/fail-closed checks, release
   record preservation, `make build`, uncached `go test ./...`, `go vet ./...`,
   and a built-binary reachability run in the proof bundle. Update S19 only to
   `implemented`; a fresh verifier must independently PASS before S20 can start.

## Design choices

- **Type-1 decision already ratified:** complete ordinary rollback to the
  immutable S02 start tree, followed by a fresh replacement slice. S19 does not
  revisit the frozen S02 semantics or reapply any selected v0.15.1 behaviour.
- **Proof is dynamic, not a 45-path assertion:** the known manifest helps a
  reviewer spot omissions, while the final first-parent walk prevents a new
  semantic path from escaping validation merely because it was authored after
  planning.
- **Release records are an immutable evidence plane:** only semantic paths are
  restored. The release root is excluded from equality only as a physical
  record-root exception, then checked separately for allowed S19-only writes.
- **Conservative provenance:** an unexpected ordinary commit, semantic merge,
  or authored/merge overlap is a failure, never a reason to subtract a path.

## Planned changes and acceptance trace

| Surface | Planned change | Acceptance criteria |
|---|---|---|
| 37 baseline-present semantic paths | Restore the exact baseline blob and mode from `e61cb190...`. | AC-01, AC-02 |
| 8 baseline-absent semantic paths | Remove the S02-added archive, implementation, and schema-fixture paths so their baseline absence is exact. | AC-01, AC-02 |
| `docs/release/.../S19-s02-v015-rollback/proof/` | Add a committed, Git-plumbing-based envelope/equality checker and capture its live output; it derives scope through S19's implementation head rather than trusting a static list. | AC-01, AC-02, AC-03 |
| `docs/release/.../S19-s02-v015-rollback/proof.md` and `proof.json` | Record the complete envelope, equality, negative checks, release-record preservation, test results, built-binary reachability, and fresh-verification handoff. | AC-01 through AC-05 |
| `docs/release/.../S19-s02-v015-rollback/status.json`, `journal.md` | Record lifecycle transitions, exact implementation head, Rule-2 boundary, and the S20 gate without changing S02's terminal status. | AC-04, AC-05 |

## Reachability and proof plan

The release-operator-facing affordance is the committed proof bundle backed by
live Git plumbing. The first proof command will run the checker at the pinned
S19 implementation head and fail on any missing, extra, mode-drifted,
blob-drifted, surviving-added, ordinary-authority, merge-overlap, or
release-record violation. A separately built `bin/sworn` invocation plus the
uncached repository suite demonstrates the restored semantic tree remains a
working Sworn checkout.

## Review pins for the fresh Captain

1. **[MECHANICAL]** The envelope derives from every first-parent ordinary commit
   through S19's final implementation head; the 45 paths cannot become a
   hard-coded exemption list.
2. **[MECHANICAL]** Equality checks modes, blob OIDs, and expected absence, with
   a whole-tree non-release backstop.
3. **[MECHANICAL]** No rollback mechanism may rewrite or relax S02's preserved
   claim, receipt, reports, proof, journal, verifier failure, or terminal
   rollback-backed deferral.
4. **[MECHANICAL]** Any semantic merge, overlap, unexpected later authority, or
   non-S19 release-record mutation fails closed rather than being excluded.
5. **[BOUNDARY]** S20 remains blocked until S19 receives an independent fresh
   verification PASS; no v0.15.1 parity or local-install work belongs here.
