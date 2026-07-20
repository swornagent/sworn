# Exact local candidate

`internal/repo` is the sole owner of measured Git repository, target, workspace,
and candidate facts. It does not trust a remote alias, current checkout, agent
report, index, or branch shorthand as identity.

The boundary is deliberately small:

1. `Discover` measures physical Git common and object directories plus the
   object format while accepting an opaque repository ID from configuration. It
   never derives or equates repository identity from a path or URL.
2. `Open` compares those live facts with the immutable binding. A different
   common directory or object format fails as binding drift.
3. `BindTarget` accepts only a Git-validated full `refs/heads/...` ref and
   resolves its exact commit and tree.
4. `Materialize` copies that tree through a private index into a new plain
   workspace outside the source worktree and Git common directory. The workspace
   contains no `.git` path and cannot directly mutate repository refs or the
   user's index.
5. `PrepareCandidate` rechecks the target, stages workspace bytes in another
   private index, writes the tree, and derives actual changed paths from the
   base-tree to candidate-tree diff. Baton literal-prefix scope is then enforced
   over every changed path; exclusions win. It publishes no ref. `Capture`
   remains the prepare-and-retain convenience for callers that do not cross a
   journal-binding boundary.
6. For a changed tree, Sworn creates a single-parent commit whose exact parent is
   the bound base. For an unchanged tree, the candidate remains the base commit;
   no artificial commit is created.
7. After a native builder result is durably bound, Store retains the verified
   commit under `refs/sworn/v1/candidates/<commit-oid>` and publishes
   `refs/sworn/v1/attempts/<invocation-id>` at the same commit. Expected-absent
   updates and exact readback reject collisions, non-commit occupants, and
   symbolic refs. `EnsureCandidate` and `EnsureAttemptCandidate` can repair
   missing refs only while the base, commit, trees, parent, and changed paths
   still match.

Git runs with system and global configuration, replacement objects, prompts,
hooks, credential helpers, external diffs, filesystem monitors, and configured
clean/smudge/process filters disabled. Candidate capture uses bounded command
output and never invokes a shell. Repositories using object alternates, grafts,
or Gitlinks fail as unsupported instead of weakening the claim.

## Current boundary

No mutating CLI command calls this package yet. Immutable process configuration
must bind the discovered repository before native execution or admission. The
[contained Linux executor](contained-executor.md) returns a bounded, quiescent,
measured writable export. The native builder validates that export immediately
before cloning the original `Workspace` binding with only its path replaced,
then prepares the exact candidate here. Candidate identity and changed paths
still come independently from Git.

Store publishes the native candidate only after its typed result is bound and
before journal success. The internal `check.local` worker then freshly
materializes that candidate through this boundary before content-bound
execution. The [atomic admission transaction](measured-submission.md)
independently rederives
the retained candidate, parent, tree, changed paths, and scope immediately
before binding its canonical record to reviewable engine state. The internal
builder boundary is complete, but no public mutation command, autonomous claim
loop, verifier adapter, or target integration invokes it yet.

Prepared objects are temporarily unreachable between result binding and Store
publication. Normal crash recovery assumes Git retains those objects during
that short window. An external immediate prune makes recovery stop with the
bound result still unknown; it cannot manufacture or substitute a candidate.
Such same-UID repository maintenance is inside the declared host trust boundary.

If a process dies after a build result is durably bound but before its effect is
completed, Store recovery calls `EnsureAttemptCandidate` through that same
configured repository. Before touching Git, it requires both request digests,
the exact plan and work attempt, configured repository/target, and candidate to
agree with the journal. Missing deterministic refs are recreated only after
every object, parent, tree, and changed-path fact revalidates. A collision or
missing/mutated Git fact leaves the effect unknown. Exact replay re-establishes
that external postcondition without duplicating the journal observation.

For an unbound native attempt, the attempt ref is the publication witness.
Under exclusive controller ownership, `ProveAttemptUnpublished` returns an
opaque live proof only when the exact ref is genuinely absent. An occupied
commit, tag/blob/tree, live or dangling symbolic ref, invalid object, or Git
error stops recovery. The builder combines that proof with exact executor and
attempt-root cleanup before Store may requeue the attempt. Legacy requests have
no such authority. See [ADR 0005](adr/0005-native-builder-recovery.md).

The target is rechecked while preparing candidate facts and admission
revalidates the immutable candidate. Git is not part of the SQLite transaction:
Store orders publication after result binding and makes it idempotent before
journal success. v1 assumes exclusive engine ownership of `refs/sworn/v1/*`
and treats a hostile concurrent same-UID repository writer as inside the engine
trust boundary. Target movement remains external reality for the later
integration compare-and-swap and reconciliation path.
